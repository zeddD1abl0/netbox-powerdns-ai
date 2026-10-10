package cli

import (
	"context"
	"crypto/rand"
	"reflect"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/logging"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/metrics"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/service"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
)

// serveClients are serve's clients for NetBox and each group's primary,
// made from the session's configuration, which reconfigure changes.
type serveClients struct {
	s      *session
	groups []groupClient
	netbox *netboxConn
	// cfg is the configuration that they were made from.
	cfg *config.Config
}

// newClients makes serve's clients, from the session's configuration.
func (s *session) newClients(ctx context.Context, groups []config.Group) (*serveClients, error) {
	nb, err := s.netbox(ctx)
	if err != nil {
		return nil, err
	}
	c := &serveClients{s: s, netbox: &netboxConn{c: nb}, groups: s.groupClients(ctx, groups), cfg: s.cfg}
	c.warn(ctx)
	return c, nil
}

// warn logs each group that has no client.
func (c *serveClients) warn(ctx context.Context) {
	for _, g := range c.groups {
		if g.err != nil {
			c.s.log.WarnContext(ctx, "a server group has no client; each refresh tries again, and reports it failed until one works",
				"group", g.cfg.Name, "err", g.err)
		}
	}
}

func (c *serveClients) close() {
	c.netbox.c.Close()
	closeClients(c.groups)
}

// reconfigure makes cfg the session's configuration, unless it is already,
// and makes new clients for NetBox, or for the primaries, if their settings
// changed, such as a timeout. It runs at the start of a refresh, so no
// request is in flight. A NetBox client that can't be made leaves the old
// one, and the next refresh tries again.
func (c *serveClients) reconfigure(ctx context.Context, cfg *config.Config) {
	s := c.s
	if cfg == c.cfg {
		return
	}
	s.cfg = cfg
	if !reflect.DeepEqual(cfg.NetBox, c.cfg.NetBox) {
		nb, err := s.netbox(ctx)
		if err != nil {
			s.log.ErrorContext(ctx, "couldn't make a NetBox client with the changed settings, so the old one stays; the next refresh tries again",
				"err", err)
			return
		}
		c.netbox.c.Close()
		c.netbox = &netboxConn{c: nb}
	}
	if cfg.PowerDNS.Timeout != c.cfg.PowerDNS.Timeout || cfg.PowerDNS.Concurrency != c.cfg.PowerDNS.Concurrency {
		groups := make([]config.Group, len(c.groups))
		for i, g := range c.groups {
			groups[i] = g.cfg
		}
		closeClients(c.groups)
		c.groups = s.groupClients(ctx, groups)
		c.warn(ctx)
	}
	c.cfg = cfg
}

// watchSettings checks the settings' version every settingsPoll, until ctx
// is done, and when it has moved on from version, reads the settings again,
// and applies them: the log's level and the refreshes' timing at once, and
// the rest, through live, at the next refresh, or the next prune (ADR-0040).
// Settings that aren't valid are logged, and serve keeps those it has.
func (s *session) watchSettings(ctx context.Context, st *store.Store, version int64, svc *service.Service, live *atomic.Pointer[config.Config]) {
	current := s.settings
	t := time.NewTicker(settingsPoll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		v, err := st.SettingsVersion(ctx)
		if err != nil {
			if ctx.Err() == nil {
				s.log.WarnContext(ctx, "couldn't check the settings' version; serve checks again soon", "err", err)
			}
			continue
		}
		if v == version {
			continue
		}
		version = v
		current = s.reloadSettings(ctx, st, current, svc, live)
	}
}

// reloadSettings reads the settings that st stores, applies them, and
// returns them, or current, if they can't be. Each reload is its own trace.
func (s *session) reloadSettings(ctx context.Context, st *store.Store, current []config.Setting, svc *service.Service,
	live *atomic.Pointer[config.Config],
) []config.Setting {
	ctx, span := s.tracer.Start(logging.WithRequestID(ctx, rand.Text()), "settings reload", trace.WithNewRoot())
	defer span.End()
	cfg, settings, _, err := s.storedConfig(ctx, st)
	if err != nil {
		span.RecordError(err)
		s.metrics.SettingsReload(metrics.ReloadFailed)
		s.log.ErrorContext(ctx, "couldn't apply the changed settings, so serve keeps those it has", "err", err)
		return current
	}
	if err := s.level.UnmarshalText([]byte(cfg.Log.Level)); err != nil {
		// The loader has checked log.level.
		s.log.ErrorContext(ctx, "couldn't apply the log level", "err", err)
	}
	svc.SetTiming(service.Timing{Interval: cfg.Drift.Interval, Timeout: cfg.Drift.Timeout, WebhookDelay: cfg.Drift.WebhookDelay})
	live.Store(cfg)
	s.metrics.SettingsReload(metrics.ReloadApplied)
	s.log.InfoContext(ctx, "applied the changed settings", "keys", changedKeys(current, settings))
	return settings
}

// changedKeys returns the keys whose value or source differs between two
// lists of settings.
func changedKeys(before, after []config.Setting) []string {
	old := map[string]config.Setting{}
	for _, st := range before {
		old[st.Key] = st
	}
	keys := []string{}
	for _, st := range after {
		if o := old[st.Key]; o.Value != st.Value || o.Source != st.Source {
			keys = append(keys, st.Key)
		}
	}
	return keys
}
