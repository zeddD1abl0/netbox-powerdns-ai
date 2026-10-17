package cli

import (
	"context"
	"crypto/rand"
	"fmt"
	"reflect"
	"slices"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/logging"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/metrics"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/secrets"
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
func (s *session) newClients(ctx context.Context) (*serveClients, error) {
	nb, err := s.netbox(ctx)
	if err != nil {
		return nil, err
	}
	c := &serveClients{s: s, netbox: &netboxConn{c: nb}, cfg: s.cfg}
	c.groups = c.update(ctx, nil, s.cfg.PowerDNS.Groups, true)
	return c, nil
}

// update returns a client for each of groups: old's, if it has one for a
// group that's unchanged, unless all is set, and a new one otherwise. It
// closes the rest of old's, and drops the metrics of the groups that are
// gone, whose requests have finished. It logs each group that has no
// client.
func (c *serveClients) update(ctx context.Context, old []groupClient, groups []config.Group, all bool) []groupClient {
	s := c.s
	byName := map[string]groupClient{}
	for _, g := range old {
		byName[g.cfg.Name] = g
	}
	out := make([]groupClient, len(groups))
	for i, g := range groups {
		if o, ok := byName[g.Name]; ok && !all && reflect.DeepEqual(o.cfg, g) {
			out[i] = o
			delete(byName, g.Name)
			continue
		}
		cl, err := s.powerdns(ctx, g)
		out[i] = groupClient{cfg: g, c: cl, err: err}
		if err != nil {
			s.log.WarnContext(ctx, "a server group has no client; each refresh tries again, and reports it failed until one works",
				"group", g.Name, "err", err)
		}
	}
	for name, o := range byName {
		if o.c != nil {
			o.c.Close()
		}
		if s.metrics != nil && !slices.ContainsFunc(groups, func(g config.Group) bool { return g.Name == name }) {
			s.metrics.DropGroup(name)
		}
	}
	return out
}

func (c *serveClients) close() {
	c.netbox.c.Close()
	closeClients(c.groups)
}

// reconfigure makes cfg the session's configuration, unless it is already,
// and makes new clients for NetBox, or for the primaries, if their settings
// changed, such as a timeout, and for the groups that are new or changed
// (ADR-0040). It runs at the start of a refresh, so no request is in
// flight. A NetBox client that can't be made leaves the old one, and the
// next refresh tries again.
func (c *serveClients) reconfigure(ctx context.Context, cfg *config.Config) {
	s := c.s
	if cfg == c.cfg {
		return
	}
	s.cfg = cfg
	prev := c.cfg
	c.cfg = cfg
	all := cfg.PowerDNS.Timeout != prev.PowerDNS.Timeout || cfg.PowerDNS.Concurrency != prev.PowerDNS.Concurrency
	if all || !reflect.DeepEqual(cfg.PowerDNS.Groups, prev.PowerDNS.Groups) {
		c.groups = c.update(ctx, c.groups, cfg.PowerDNS.Groups, all)
	}
	if reflect.DeepEqual(cfg.NetBox, prev.NetBox) {
		return
	}
	nb, err := s.netbox(ctx)
	if err != nil {
		s.log.ErrorContext(ctx, "couldn't make a NetBox client with the changed settings, so the old one stays; the next refresh tries again",
			"err", err)
		// The clients are as cfg says, but NetBox's, which the next
		// refresh tries again.
		kept := *cfg
		kept.NetBox = prev.NetBox
		c.cfg = &kept
		return
	}
	c.netbox.c.Close()
	c.netbox = &netboxConn{c: nb}
}

// A servedGroup is a group that serve compares, with its API key, and its
// ID in the database.
type servedGroup struct {
	id  int64
	cfg config.Group
}

// servedGroups returns the groups that serve compares, and their version:
// file, the config file's groups, which st must have mirrored, then the CLI's
// that st stores, by name, their API keys opened with database.master_key
// (ADR-0040, ADR-0041).
func (s *session) servedGroups(ctx context.Context, st *store.Store, file []config.Group) ([]servedGroup, int64, error) {
	stored, version, err := st.Groups(ctx)
	if err != nil {
		return nil, 0, err
	}
	ids := map[string]int64{}
	var cli []servedGroup
	var mk secrets.MasterKey
	for _, g := range stored {
		if g.Group.ManagedBy == config.ManagedByFile {
			ids[g.Group.Name] = g.ID
			continue
		}
		if !mk.IsSet() {
			if mk, err = s.masterKey(); err != nil {
				return nil, 0, err
			}
		}
		key, err := secrets.Open(mk, store.GroupPlace(g.ID), g.APIKey)
		if err != nil {
			return nil, 0, fmt.Errorf("the database stores server group %s's API key, which doesn't open: %w", g.Group.Name, err)
		}
		cfg := g.Group
		cfg.Primary.APIKey = config.NewSecret(key)
		cli = append(cli, servedGroup{id: g.ID, cfg: cfg})
	}
	out := make([]servedGroup, 0, len(file)+len(cli))
	for _, g := range file {
		out = append(out, servedGroup{id: ids[g.Name], cfg: g})
	}
	return append(out, cli...), version, nil
}

// withGroups returns a copy of cfg that compares groups.
func withGroups(cfg *config.Config, groups []servedGroup) *config.Config {
	out := *cfg
	out.PowerDNS.Groups = make([]config.Group, len(groups))
	for i, g := range groups {
		out.PowerDNS.Groups[i] = g.cfg
	}
	return &out
}

// serviceGroups returns groups as the service shows them.
func serviceGroups(groups []servedGroup) []service.Group {
	out := make([]service.Group, len(groups))
	for i, sg := range groups {
		g := sg.cfg
		out[i] = service.Group{ID: sg.id, Name: g.Name, URL: g.Primary.URL, Views: g.Views, DriftPolicy: g.DriftPolicy, ManagedBy: g.ManagedBy}
	}
	return out
}

// groupChanges are the differences between two lists of served groups, by
// name.
type groupChanges struct {
	Added, Changed, Removed, Renamed []string
}

// diffGroups returns how next differs from prev, matching groups by their
// IDs. A renamed group is in Renamed, as old->new, and, if its fields
// changed too, in Changed, under its new name.
func diffGroups(prev, next []servedGroup) groupChanges {
	var d groupChanges
	before := map[int64]servedGroup{}
	for _, g := range prev {
		before[g.id] = g
	}
	for _, g := range next {
		o, ok := before[g.id]
		if !ok {
			d.Added = append(d.Added, g.cfg.Name)
			continue
		}
		delete(before, g.id)
		if o.cfg.Name != g.cfg.Name {
			d.Renamed = append(d.Renamed, o.cfg.Name+"->"+g.cfg.Name)
			o.cfg.Name = g.cfg.Name
		}
		if !reflect.DeepEqual(o.cfg, g.cfg) {
			d.Changed = append(d.Changed, g.cfg.Name)
		}
	}
	for _, g := range prev {
		if _, gone := before[g.id]; gone {
			d.Removed = append(d.Removed, g.cfg.Name)
		}
	}
	return d
}

// A watcher applies the settings and the groups that the database stores to
// a running serve, when their versions move on (ADR-0040).
type watcher struct {
	s    *session
	st   *store.Store
	svc  *service.Service
	live *atomic.Pointer[config.Config]
	// file are the config file's groups, as serve loaded them at its start.
	file []config.Group
	// The settings and the groups applied, and their versions.
	settings                       []config.Setting
	groups                         []servedGroup
	settingsVersion, groupsVersion int64
}

// run checks the settings' and the groups' versions every settingsPoll,
// until ctx is done, and when one has moved on, reads them again and
// applies them. Settings or groups that can't be applied are logged, and
// serve keeps those it has, and tries again at the next check.
func (w *watcher) run(ctx context.Context) {
	t := time.NewTicker(settingsPoll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if v, ok := w.version(ctx, "settings", w.st.SettingsVersion); ok && v != w.settingsVersion && w.reloadSettings(ctx) {
			w.settingsVersion = v
		}
		if v, ok := w.version(ctx, "server groups", w.st.GroupsVersion); ok && v != w.groupsVersion && w.reloadGroups(ctx) {
			w.groupsVersion = v
		}
	}
}

// version returns a version, read by read, or logs why it couldn't be.
func (w *watcher) version(ctx context.Context, what string, read func(context.Context) (int64, error)) (int64, bool) {
	v, err := read(ctx)
	if err != nil {
		if ctx.Err() == nil {
			w.s.log.WarnContext(ctx, "couldn't check whether the database's configuration changed; serve checks again soon",
				"of", what, "err", err)
		}
		return 0, false
	}
	return v, true
}

// reloadSettings reads the settings that the database stores, and applies
// them: the log's level and the refreshes' timing at once, and the rest,
// through live, at the next refresh, or the next prune. It reports whether
// it applied them. Each reload is its own trace.
func (w *watcher) reloadSettings(ctx context.Context) bool {
	s := w.s
	ctx, span := s.tracer.Start(logging.WithRequestID(ctx, rand.Text()), "settings reload", trace.WithNewRoot())
	defer span.End()
	cfg, settings, _, err := s.storedConfig(ctx, w.st)
	if err != nil {
		if ctx.Err() != nil {
			return false
		}
		span.RecordError(err)
		s.metrics.SettingsReload(metrics.ReloadFailed)
		s.log.ErrorContext(ctx, "couldn't apply the changed settings, so serve keeps those it has, and tries again soon", "err", err)
		return false
	}
	if err := s.level.UnmarshalText([]byte(cfg.Log.Level)); err != nil {
		// The loader has checked log.level.
		s.log.ErrorContext(ctx, "couldn't apply the log level", "err", err)
	}
	w.svc.SetTiming(service.Timing{Interval: cfg.Drift.Interval, Timeout: cfg.Drift.Timeout, WebhookDelay: cfg.Drift.WebhookDelay})
	w.svc.SetWebhooks(cfg.NetBox.WebhookSecret.IsSet())
	// The loader's groups are the config file's alone.
	w.live.Store(withGroups(cfg, w.groups))
	s.metrics.SettingsReload(metrics.ReloadApplied)
	s.log.InfoContext(ctx, "applied the changed settings", "keys", changedKeys(w.settings, settings))
	w.settings = settings
	return true
}

// reloadGroups reads the groups that the database stores, and applies them:
// the service shows them at once, and compares a new or changed one at
// once, and the next refresh makes their clients. It reports whether it
// applied them. Each reload is its own trace.
func (w *watcher) reloadGroups(ctx context.Context) bool {
	s := w.s
	ctx, span := s.tracer.Start(logging.WithRequestID(ctx, rand.Text()), "server groups reload", trace.WithNewRoot())
	defer span.End()
	groups, _, err := s.servedGroups(ctx, w.st, w.file)
	if err != nil {
		if ctx.Err() != nil {
			return false
		}
		span.RecordError(err)
		s.metrics.SettingsReload(metrics.ReloadFailed)
		s.log.ErrorContext(ctx, "couldn't apply the changed server groups, so serve keeps those it has, and tries again soon", "err", err)
		return false
	}
	d := diffGroups(w.groups, groups)
	// The next refresh reads live, so it's stored before the service asks
	// for one.
	w.live.Store(withGroups(w.live.Load(), groups))
	w.svc.SetGroups(serviceGroups(groups))
	if len(d.Added) > 0 || len(d.Changed) > 0 {
		w.svc.RefreshNow()
	}
	w.groups = groups
	s.metrics.SettingsReload(metrics.ReloadApplied)
	s.log.InfoContext(ctx, "applied the changed server groups",
		"added", nonNil(d.Added), "changed", nonNil(d.Changed), "removed", nonNil(d.Removed), "renamed", nonNil(d.Renamed))
	return true
}

// nonNil returns s, or an empty slice if it's nil, so that a log shows [].
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
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
