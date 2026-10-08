package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/api"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/logging"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/metrics"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/service"
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
			"  /livez    200 unless no refresh has started for longer than one can take\n" +
			"  /readyz   200 once the first refresh has finished\n" +
			"  /status   the service's state, as text, or as JSON with ?json=1\n" +
			"  /metrics  the drift, refresh and request metrics, for Prometheus\n\n" +
			"With netbox.webhook_secret set, NetBox's signed webhooks at /api/netbox-events\n" +
			"refresh the zones they name, once they stop coming for drift.webhook_delay.\n\n" +
			"Each refresh is its own trace, exported if otlp.endpoint is set. nbpdns only\n" +
			"reads, and changes nothing. It stops on SIGINT or SIGTERM, and exits 0.",
		Args: usageArgs(cobra.NoArgs),
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return a.run(cmd, func(ctx context.Context, s *session) error {
			groups, err := s.groups("")
			if err != nil {
				return err
			}
			s.metrics = metrics.New(version.Get())
			nb, err := s.netbox(ctx)
			if err != nil {
				return err
			}
			defer nb.Close()
			clients := s.groupClients(ctx, groups)
			defer closeClients(clients)
			for _, c := range clients {
				if c.err != nil {
					s.log.WarnContext(ctx, "a server group has no client; each refresh tries again, and reports it failed until one works",
						"group", c.cfg.Name, "err", c.err)
				}
			}
			nbc := &netboxConn{c: nb}
			primaries := make([]service.Group, len(groups))
			for i, g := range groups {
				primaries[i] = service.Group{Name: g.Name, URL: g.Primary.URL, Views: g.Views, DriftPolicy: g.DriftPolicy}
			}
			svc := service.New(service.Options{
				Refresh: func(ctx context.Context, zones []drift.ZoneRef) (drift.Report, error) {
					s.retryClients(ctx, clients)
					// The API serves NetBox's records too (ADR-0033), and a
					// webhook's refresh compares only its zones (ADR-0035).
					return s.compare(ctx, nbc, clients, drift.Options{ReadNetBox: true, Zones: zones})
				},
				Interval:     s.cfg.Drift.Interval,
				Timeout:      s.cfg.Drift.Timeout,
				WebhookDelay: s.cfg.Drift.WebhookDelay,
				Webhooks:     s.cfg.NetBox.WebhookSecret.IsSet(),
				Log:          s.log,
				Tracer:       s.tracer,
				Metrics:      s.metrics,
				Version:      version.Get(),
				NetBoxURL:    s.cfg.NetBox.URL,
				Groups:       primaries,
				OTLP:         service.OTLP{Endpoint: s.cfg.OTLP.Endpoint, Protocol: s.cfg.OTLP.Protocol},
			})
			return serve(ctx, s, svc)
		})
	}
	return cmd
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

// serve listens at server.listen, and runs svc until ctx is canceled or the
// listener fails. Then it lets the requests in flight finish.
func serve(ctx context.Context, s *session, svc *service.Service) error {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.cfg.Server.Listen)
	if err != nil {
		return fmt.Errorf("server.listen: %w", err)
	}
	// The API (ADR-0033) is under /api, beside the service's own endpoints.
	mux := http.NewServeMux()
	// NetBox's webhooks queue the service's zone refreshes (ADR-0035).
	mux.Handle("/api/", api.New(api.Options{
		Source: svc, Log: s.log, Tracer: s.tracer, Metrics: s.metrics, PublicURL: s.cfg.Server.PublicURL,
		WebhookSecret: s.cfg.NetBox.WebhookSecret, Events: svc,
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
	svc.Run(runCtx)

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
