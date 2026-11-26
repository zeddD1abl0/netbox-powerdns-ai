package cli

import (
	"context"
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/trace"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/api"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/logging"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/metrics"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/service"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/version"
)

// shutdownTimeout is how long `nbpdns serve` lets requests in flight finish
// when it stops.
const shutdownTimeout = 10 * time.Second

func newServeCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run continuously: refresh the drift report on a schedule, and serve its status and metrics",
		Long: "Refresh the drift report on a schedule, every drift.interval, as nbpdns drift\n" +
			"would make it, and keep each server group's last-known state when its primary,\n" +
			"or NetBox, can't be read. Serve, at server.listen:\n\n" +
			"  /livez    200 unless no refresh has started for longer than one can take,\n" +
			"            or, on a follower, it hasn't tried to lead for a minute\n" +
			"  /readyz   200 once it serves a refresh that finished: one from the\n" +
			"            database, or else the first, once the database has it\n" +
			"  /status   the service's state, as text, or as JSON with ?json=1\n" +
			"  /metrics  the drift, refresh and request metrics, for Prometheus\n\n" +
			"It keeps its state in the database at database.url, which it creates, and\n" +
			"migrates, when it starts: the audit trail, each refresh, each zone's drift\n" +
			"history, and NetBox's zones. It serves a refresh once the database has it,\n" +
			"and loads the last one from it at start, which it serves until its next\n" +
			"refresh replaces it. On SQLite, only one serve may use a database at a time.\n" +
			"On PostgreSQL, several can, as replicas: one of them, the leader, refreshes\n" +
			"and prunes, and another takes over within about 30 seconds if it stops, dies\n" +
			"or is cut off. Every replica serves the refreshes that the database holds.\n\n" +
			"It compares the config file's server groups, which it mirrors into the\n" +
			"database at start, and those that nbpdns groups added. It checks the\n" +
			"database every database.poll_interval, or at once with database.listen on\n" +
			"PostgreSQL, and applies the changed groups and runtime settings, and loads\n" +
			"the leader's refreshes.\n\n" +
			"With netbox.webhook_secret set, NetBox's signed webhooks at /api/netbox-events\n" +
			"refresh the zones they name, once they stop coming for drift.webhook_delay.\n\n" +
			"Each refresh is its own trace, exported if otlp.endpoint is set. nbpdns only\n" +
			"reads, and changes nothing. It stops on SIGINT or SIGTERM, and exits 0.",
		Args: usageArgs(cobra.NoArgs),
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return a.run(cmd, func(ctx context.Context, s *session) (err error) {
			s.metrics = metrics.New(version.Get())
			host, _ := os.Hostname()
			me := store.Replica{Name: host, ID: rand.Text()}
			st, ss, settingsVersion, closeStore, err := s.openServeStore(ctx, me)
			if err != nil {
				return err
			}
			defer func() { closeStore(err) }()
			// The config file's groups, which openServeStore mirrored, and the
			// CLI's, which follow them (ADR-0040).
			file := s.cfg.PowerDNS.Groups
			groups, groupsVersion, err := s.servedGroups(ctx, st, file)
			if err != nil {
				return err
			}
			if len(groups) == 0 {
				s.log.WarnContext(ctx, "there are no server groups to compare; declare them in the config file, or add them with nbpdns groups add")
			}
			s.cfg = withGroups(s.cfg, groups)
			c, err := s.newClients(ctx)
			if err != nil {
				return err
			}
			defer c.close()
			// live is the configuration as the settings and the groups change
			// it: serve's background loops read it, and each refresh applies
			// it.
			var live atomic.Pointer[config.Config]
			live.Store(s.cfg)
			svc := service.New(service.Options{
				Refresh: func(ctx context.Context, zones []drift.ZoneRef) (drift.Report, error) {
					// Refreshes never overlap, so this is the one place that
					// changes the session's configuration and clients.
					c.reconfigure(ctx, live.Load())
					s.retryClients(ctx, c.groups)
					// The API serves NetBox's records too (ADR-0033), and a
					// webhook's refresh compares only its zones (ADR-0036).
					return s.compare(ctx, c.netbox, c.groups, drift.Options{ReadNetBox: true, Zones: zones})
				},
				Interval:     s.cfg.Drift.Interval,
				Timeout:      s.cfg.Drift.Timeout,
				WebhookDelay: s.cfg.Drift.WebhookDelay,
				Webhooks:     s.cfg.NetBox.WebhookSecret.IsSet(),
				History:      st,
				Database:     s.databaseInfo(st),
				Replica:      me,
				Elected:      ss != nil,
				Log:          s.log,
				Tracer:       s.tracer,
				Metrics:      s.metrics,
				Version:      version.Get(),
				NetBoxURL:    s.cfg.NetBox.URL,
				Groups:       serviceGroups(groups),
				OTLP:         service.OTLP{Endpoint: s.cfg.OTLP.Endpoint, Protocol: s.cfg.OTLP.Protocol},
			})
			// The last reports are served at once (ADR-0042), and the first
			// refresh starts as serve listens, on the leader.
			svc.Restore(ctx)
			// The leader's work: the refreshes and the prune (ADR-0047).
			lead := func(ctx context.Context) {
				var prune sync.WaitGroup
				prune.Go(func() { s.prune(ctx, st, &live) })
				svc.Run(ctx)
				prune.Wait()
			}
			// The change feed (ADR-0048) and its watcher run beside the
			// refreshes, which change the session's configuration, so they
			// have their own copy of the session. They read only the
			// database.* keys from its configuration, bootstrap keys, which
			// don't change while serve runs.
			ws := *s
			w := &watcher{s: &ws, st: st, svc: svc, live: &live, file: file, settings: s.settings, groups: groups,
				settingsVersion: settingsVersion, groupsVersion: groupsVersion}
			f := newFeed(&ws, st, w, ss != nil)
			// On SQLite, serve leads, under its flock. On PostgreSQL, the
			// replicas elect the leader, at the feed's checks.
			run := lead
			if ss != nil {
				run = (&election{s: s, st: st, svc: svc, me: me, ss: ss, lead: lead, polls: f.polls}).run
			} else {
				s.metrics.Leader(true, time.Now())
			}
			// The feed uses the database, so it stops before closeStore
			// records serve's stop and closes it.
			bgCtx, stopBackground := context.WithCancel(ctx)
			var background sync.WaitGroup
			defer func() {
				stopBackground()
				background.Wait()
			}()
			background.Go(func() { f.run(bgCtx) })
			if s.cfg.Database.Listen {
				background.Go(func() { f.listen(bgCtx) })
			}
			return serve(ctx, s, svc, st, func() config.Secret { return live.Load().NetBox.WebhookSecret }, run)
		})
	}
	return cmd
}

// pruneInterval is how often serve prunes the audit trail and the drift
// history.
const pruneInterval = 24 * time.Hour

// openServeStore opens the database for me, this serve, under its lock
// (ADR-0038): on SQLite, the flock, so that one serve uses it; on
// PostgreSQL, the serve lock, shared with the other replicas, in the
// replica's session, which it returns for the election (ADR-0047). Either
// way, the commands that need no serve to run, such as db migrate, refuse
// meanwhile. It migrates the database, applies its settings, mirrors the
// config file's groups into it (ADR-0040), and records serve's start in the
// audit trail (ADR-0039). Its close function records serve's stop, with err,
// the error it stopped with, if any, then releases the lock and closes the
// database.
func (s *session) openServeStore(ctx context.Context, me store.Replica) (*store.Store, *store.Session, int64, func(err error), error) {
	st, ss, release, err := s.openServing(ctx, me)
	if err != nil {
		return nil, nil, 0, nil, err
	}
	fail := func(err error) (*store.Store, *store.Session, int64, func(error), error) {
		release()
		_ = st.Close()
		return nil, nil, 0, nil, err
	}
	if _, err := s.migrate(ctx, st); err != nil {
		return fail(err)
	}
	info, err := st.Info(ctx)
	if err != nil {
		return fail(err)
	}
	if st.Backend() == store.PostgreSQL {
		s.log.InfoContext(ctx, "opened the database", "backend", st.Backend(), "url", s.cfg.Database.ShownURL(),
			"tls", st.TLS(), "release", st.ServerRelease(), "database_id", info.ID)
	} else {
		s.log.InfoContext(ctx, "opened the database", "backend", st.Backend(), "path", st.Path(), "database_id", info.ID)
	}
	// The runtime settings that the database stores (ADR-0040), which the
	// configuration's fingerprint covers.
	settingsVersion, err := s.applyStored(ctx, st)
	if err != nil {
		return fail(err)
	}
	// The config file's groups, mirrored as managed_by file; a name that the
	// CLI holds stops serve.
	events, err := st.MirrorFileGroups(ctx, s.cfg.PowerDNS.Groups, audit.File(s.loader.ConfigFile()))
	if err != nil {
		return fail(err)
	}
	s.metrics.AuditEvents(events)
	if len(events) > 0 {
		s.log.InfoContext(ctx, "mirrored the config file's changed server groups into the database", "changes", len(events))
	}
	target := audit.Target{Type: "service", ID: me.Name}
	after, err := audit.Value(map[string]string{"version": version.Get().Version, "config_fingerprint": s.configFingerprint()})
	if err != nil {
		return fail(err)
	}
	e, err := st.Audit(ctx, audit.Event{Actor: audit.System(), Action: audit.ServiceStarted, Target: target, After: after}.WithContext(ctx))
	if err != nil {
		return fail(err)
	}
	s.metrics.AuditEvents([]audit.Event{e})
	start := time.Now()
	return st, ss, settingsVersion, func(serveErr error) {
		// The signal that stopped serve canceled ctx, but its stop is still
		// recorded.
		sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()
		v := map[string]any{"uptime_seconds": int64(time.Since(start).Seconds())}
		if serveErr != nil {
			v["error"] = serveErr.Error()
		}
		after, err := audit.Value(v)
		var e audit.Event
		if err == nil {
			e, err = st.Audit(sctx, audit.Event{Actor: audit.System(), Action: audit.ServiceStopped, Target: target, After: after}.WithContext(sctx))
		}
		if err != nil {
			s.metrics.DBWriteFailure(metrics.WriteAudit)
			s.log.ErrorContext(sctx, "couldn't record serve's stop in the audit trail", "err", err)
		} else {
			s.metrics.AuditEvents([]audit.Event{e})
		}
		release()
		_ = st.Close()
	}, nil
}

// openServing opens the database at database.url for me, under serve's lock:
// on SQLite, the flock, which comes first, so that a second serve changes
// nothing; on PostgreSQL, the serve lock, shared, in the session that me
// joins the replicas with, which it returns. Its release function releases
// the lock; the election may have ended that session already, and joined
// with another, which it ends itself.
func (s *session) openServing(ctx context.Context, me store.Replica) (*store.Store, *store.Session, func(), error) {
	if s.cfg.Database.SQLitePath() != "" {
		st, lock, err := s.openLocked(ctx, s.createStore)
		if err != nil {
			return nil, nil, nil, err
		}
		return st, nil, func() { _ = lock.Close() }, nil
	}
	st, err := s.createStore(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	ss, err := st.Join(ctx, me)
	if err != nil {
		_ = st.Close()
		return nil, nil, nil, err
	}
	return st, ss, ss.Close, nil
}

// databaseInfo describes st, the database at database.url, for /status.
func (s *session) databaseInfo(st *store.Store) service.DatabaseInfo {
	d := service.DatabaseInfo{Backend: st.Backend(), URL: s.cfg.Database.ShownURL(), Path: st.Path()}
	if st.Backend() == store.PostgreSQL {
		encrypted := st.TLS()
		d.TLS = &encrypted
	}
	return d
}

// configFingerprint returns the SHA-384 of the configuration's settings,
// with their sources, and its server groups, each secret only as set or not,
// so that the audit trail shows when the configuration changed between two
// starts.
func (s *session) configFingerprint() string {
	// Settings and groups always encode.
	b, _ := json.Marshal(struct {
		Settings []config.Setting
		Groups   []config.Group
	}{s.settings, s.cfg.PowerDNS.Groups})
	sum := sha512.Sum384(b)
	return "sha384:" + hex.EncodeToString(sum[:])
}

// prune deletes the audit events older than audit.retention_days, and the
// drift history older than drift.history_retention_days, as live has them,
// at once and then daily, until ctx is done. Each prune is its own trace.
func (s *session) prune(ctx context.Context, st *store.Store, live *atomic.Pointer[config.Config]) {
	for {
		cfg := live.Load()
		pctx, span := s.tracer.Start(logging.WithRequestID(ctx, rand.Text()), "database prune", trace.WithNewRoot())
		fail := func(err error) {
			if ctx.Err() != nil {
				return
			}
			span.RecordError(err)
			s.metrics.DBWriteFailure(metrics.WritePrune)
			s.log.ErrorContext(pctx, "couldn't prune the database; serve tries again in a day", "err", err)
		}
		days := cfg.Audit.RetentionDays
		n, events, err := st.PruneAudit(pctx, time.Now().AddDate(0, 0, -days), days)
		switch {
		case err != nil:
			fail(err)
		case n > 0:
			s.metrics.AuditEvents(events)
			s.log.InfoContext(pctx, "pruned the audit trail", "deleted", n, "retention_days", days)
		}
		days = cfg.Drift.HistoryRetentionDays
		refreshes, rows, err := st.PruneDriftHistory(pctx, time.Now().AddDate(0, 0, -days))
		switch {
		case err != nil:
			fail(err)
		case refreshes > 0 || rows > 0:
			s.log.InfoContext(pctx, "pruned the drift history", "refreshes", refreshes, "zone_rows", rows, "retention_days", days)
		}
		span.End()
		select {
		case <-ctx.Done():
			return
		case <-time.After(pruneInterval):
		}
	}
}

// retryClients tries again to make a client for each group that has none,
// such as one whose certificate file couldn't be read when serve started.
// Refreshes never overlap, so it can change clients.
func (s *session) retryClients(ctx context.Context, clients []groupClient) {
	for i := range clients {
		g := &clients[i]
		if g.c != nil {
			continue
		}
		g.c, g.err = s.powerdns(ctx, g.cfg)
		if g.err == nil {
			s.log.InfoContext(ctx, "a server group has a client now", "group", g.cfg.Name)
		}
	}
}

// serve listens at server.listen, and calls run, which runs svc, or the
// election that runs it on the leader, until ctx is canceled or the listener
// fails, with the API serving history's drift history. Then it lets the
// requests in flight finish.
func serve(ctx context.Context, s *session, svc *service.Service, history api.History, webhookSecret func() config.Secret,
	run func(ctx context.Context)) error {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.cfg.Server.Listen)
	if err != nil {
		return fmt.Errorf("server.listen: %w", err)
	}
	// The API (ADR-0033) is under /api, beside the service's own endpoints.
	mux := http.NewServeMux()
	// NetBox's webhooks queue the service's zone refreshes (ADR-0036).
	mux.Handle("/api/", api.New(api.Options{
		Source: svc, Log: s.log, Tracer: s.tracer, Metrics: s.metrics, PublicURL: s.cfg.Server.PublicURL,
		LiveWebhookSecret: webhookSecret, Events: svc, History: history,
	}))
	mux.Handle("/", svc.Handler())
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(logging.Bound(s.log.Handler(), ctx), slog.LevelWarn),
	}
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	served := make(chan error, 1)
	go func() {
		served <- srv.Serve(ln)
		stop()
	}()
	s.log.InfoContext(ctx, "serving", "address", ln.Addr().String(),
		"interval_seconds", s.cfg.Drift.Interval.Seconds(), "groups", len(s.cfg.PowerDNS.Groups))
	run(runCtx)

	s.log.InfoContext(ctx, "shutting down")
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		return fmt.Errorf("stopping the HTTP server: %w", err)
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serving at %s: %w", ln.Addr(), err)
	}
	return nil
}
