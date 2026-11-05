package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
)

// testStore returns a new, migrated database in a test's directory, which
// stores the groups site-a and site-b, as serve's start would.
func testStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "nbpdns.db"), store.Options{Create: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.Migrate(t.Context(), "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.MirrorFileGroups(t.Context(), []config.Group{{Name: "site-a"}, {Name: "site-b"}}, audit.File("nbpdns.yaml")); err != nil {
		t.Fatal(err)
	}
	return st
}

var twoGroups = []Group{{Name: "site-a", URL: "https://a"}, {Name: "site-b", URL: "https://b"}}

// returning is a RefreshFunc that returns r.
func returning(r drift.Report) RefreshFunc {
	return func(context.Context, []drift.ZoneRef) (drift.Report, error) { return r, nil }
}

// TestRefreshesAreKept refreshes twice, and finds each refresh, its
// reports, and the zone's change of state, in the history.
func TestRefreshesAreKept(t *testing.T) {
	st := testStore(t)
	r := report(group("site-a", drift.StateInSync), failedGroup("site-b"))
	s, logs, _ := testService(t, Options{Groups: twoGroups, History: st, Database: DatabaseInfo{Backend: "sqlite", Path: st.Path()}, Refresh: func(context.Context, []drift.ZoneRef) (drift.Report, error) {
		return r, nil
	}})
	ctx := t.Context()
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	s.refresh(ctx, t0, batch{})
	r = report(group("site-a", drift.StateDrift), group("site-b", drift.StateInSync))
	s.refresh(ctx, t0.Add(time.Minute), batch{})

	got, err := st.LoadDrift(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Last == nil || got.Last.Kind != store.RefreshFull || got.Last.Cause != store.CauseSchedule || got.Last.Outcome != "complete" {
		t.Errorf("the last refresh = %+v, want a complete, scheduled full refresh", got.Last)
	}
	a, b := got.Groups["site-a"], got.Groups["site-b"]
	if a.Report == nil || a.Report.Zones[0].State != drift.StateDrift || b.Report == nil || b.Status != drift.StatusOK {
		t.Errorf("the kept groups = %+v and %+v, want both read, and site-a's zone drifted", a, b)
	}
	events, err := st.ListAudit(ctx, store.AuditFilter{Action: audit.ZoneDriftChanged})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Target.ID != "site-a/a.example." || len(events[0].TraceID) != 32 {
		t.Errorf("zone events = %+v, want site-a/a.example.'s drift, with the refresh's trace", events)
	}
	if m := metricsText(t, s); !strings.Contains(m, `nbpdns_audit_events_total{action="zone.drift_changed"} 1`) {
		t.Errorf("the metrics don't count the zone's event:\n%s", m)
	}
	if !strings.Contains(logs.String(), `"msg":"wrote the refresh to the drift history"`) {
		t.Errorf("no log of the history's write:\n%s", logs)
	}
}

// TestRestore restores what one service kept into another, which is ready,
// and serves it, before it refreshes.
func TestRestore(t *testing.T) {
	st := testStore(t)
	ctx := t.Context()
	t0 := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	first, _, _ := testService(t, Options{Groups: twoGroups, History: st, now: func() time.Time { return t0 },
		Refresh: returning(report(group("site-a", drift.StateDrift, drift.StateInSync), failedGroup("site-b")))})
	first.refresh(ctx, t0, batch{})

	// The second service doesn't serve site-b, and serves site-c, which was
	// never read.
	groups := []Group{{Name: "site-a", URL: "https://a"}, {Name: "site-c", URL: "https://c"}}
	s, logs, _ := testService(t, Options{Groups: groups, History: st, Database: DatabaseInfo{Backend: "sqlite", Path: st.Path()}, Refresh: returning(report())})
	if s.Status().Ready {
		t.Fatal("ready before Restore")
	}
	s.Restore(ctx)
	status := s.Status()
	if !status.Ready || !status.Database.Restored || status.Database.RestoredAsOf == nil || !status.Database.RestoredAsOf.Equal(t0) {
		t.Errorf("after Restore, status = ready %v, database %+v, want ready, restored as of %v", status.Ready, status.Database, t0)
	}
	if !status.Live {
		t.Error("after Restore, not live: the restored refresh, an hour old, counted for liveness")
	}
	if lr := status.Schedule.LastRefresh; lr == nil || !lr.Finished.Equal(t0) || lr.Started.After(lr.Finished) || lr.Outcome != "incomplete" {
		t.Errorf("after Restore, the last refresh = %+v, want the one before the restart", lr)
	}
	a, c := status.Groups[0], status.Groups[1]
	if a.Status != drift.StatusOK || a.LastSuccess == nil || !a.LastSuccess.Equal(t0) || a.Counts.Drift != 1 || len(a.DriftedZones) != 1 {
		t.Errorf("site-a = %+v, want its restored report", a)
	}
	if c.Status != statusUnknown {
		t.Errorf("site-c = %+v, want unknown", c)
	}
	if v, ok := s.Group("site-b"); ok {
		t.Errorf("site-b, which the service doesn't serve, = %+v", v)
	}
	m := s.o.Metrics
	if v := testutil.ToFloat64(m.ZoneDrifted.WithLabelValues("site-a", "a.example.", drift.StateDrift)); v != 1 {
		t.Errorf("after Restore, nbpdns_drift_zone_drifted for a.example. = %v, want 1", v)
	}
	if v := testutil.ToFloat64(m.GroupUp.WithLabelValues("site-a")); v != 1 {
		t.Errorf("after Restore, nbpdns_server_group_up for site-a = %v, want 1", v)
	}
	if !strings.Contains(logs.String(), `"msg":"loaded the last drift reports from the database, so the service is ready"`) {
		t.Errorf("no log of the restore:\n%s", logs)
	}
}

func TestRestoreWithNothingToRestore(t *testing.T) {
	st := testStore(t)
	s, _, _ := testService(t, Options{Groups: twoGroups, History: st, Refresh: returning(report())})
	s.Restore(t.Context())
	if status := s.Status(); status.Ready || status.Database.Restored {
		t.Errorf("after Restore of an empty history, status = ready %v, database %+v, want neither", status.Ready, status.Database)
	}
}

// failingHistory is a history whose writes fail, as a full disk's would.
type failingHistory struct{}

func (failingHistory) RecordRefresh(context.Context, store.RefreshRecord) (store.Recorded, error) {
	return store.Recorded{}, errors.New("disk I/O error")
}

func (failingHistory) LoadDrift(context.Context, int64) (store.DriftState, error) {
	return store.DriftState{}, errors.New("disk I/O error")
}

// TestHistoryWriteFailure checks that a refresh whose history can't be
// written isn't served, and that the failure is counted, logged and shown
// (ADR-0048).
func TestHistoryWriteFailure(t *testing.T) {
	s, logs, _ := testService(t, Options{Groups: twoGroups, History: failingHistory{}, Database: DatabaseInfo{Backend: "sqlite", Path: "/x/nbpdns.db"},
		Refresh: returning(report(group("site-a", drift.StateDrift), group("site-b", drift.StateInSync)))})
	s.Restore(t.Context())
	s.refresh(t.Context(), time.Now(), batch{})
	status := s.Status()
	if status.Ready || status.Groups[0].Status != statusUnknown || status.Schedule.LastRefresh != nil || status.NetBox.Up != nil {
		t.Errorf("status = %+v, want nothing served", status)
	}
	if status.Schedule.Refreshes.Complete != 1 {
		t.Errorf("refreshes = %+v, want the one that ran counted", status.Schedule.Refreshes)
	}
	d := status.Database
	if d.HistoryWriteFailures != 1 || d.LastWriteFailure == nil || !strings.Contains(d.LastWriteError, "disk I/O error") {
		t.Errorf("database = %+v, want one failure, with its error", d)
	}
	m := metricsText(t, s)
	if !strings.Contains(m, `nbpdns_db_write_failures_total{op="history"} 1`) {
		t.Errorf("the metrics don't count the failure:\n%s", m)
	}
	if strings.Contains(m, "nbpdns_drift_zones{") || strings.Contains(m, "nbpdns_drift_last_refresh_timestamp_seconds ") {
		t.Errorf("the metrics show the refresh that wasn't written:\n%s", m)
	}
	for _, want := range []string{"couldn't load the last drift reports from the database",
		"couldn't write the refresh to the drift history, so no replica serves it"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("no %q in the logs:\n%s", want, logs)
		}
	}
	if code, body := get(t, s, http.MethodGet, "/readyz"); code != http.StatusServiceUnavailable || !strings.Contains(body, "could be written") {
		t.Errorf("/readyz = %d %q, want 503, saying that no refresh could be written", code, body)
	}
	var page strings.Builder
	if err := writeStatus(&page, status); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page.String(), "write failures: 1 refreshes couldn't be written to the drift history") {
		t.Errorf("the status page doesn't show the failure:\n%s", page.String())
	}
}

// flakyHistory is a store whose refreshes can't be written while failing
// is set.
type flakyHistory struct {
	*store.Store
	failing atomic.Bool
}

func (f *flakyHistory) RecordRefresh(ctx context.Context, r store.RefreshRecord) (store.Recorded, error) {
	if f.failing.Load() {
		return store.Recorded{}, errors.New("disk I/O error")
	}
	return f.Store.RecordRefresh(ctx, r)
}

// deployment returns the facts of st that are the deployment's, which
// every replica shows alike (ADR-0048).
func deployment(st Status) any {
	return struct {
		LastRefresh  *Refresh
		LastComplete *time.Time
		NetBox       NetBox
		Groups       []GroupInfo
	}{st.Schedule.LastRefresh, st.Schedule.LastCompleteRefresh, st.NetBox, st.Groups}
}

// servedBy returns what s serves of its deployment, as JSON: the
// deployment's facts in its status, each group's report, NetBox's zones, and
// the drift metrics.
func servedBy(t *testing.T, s *Service) string {
	t.Helper()
	b, err := json.MarshalIndent(struct {
		Status  any
		Reports []*drift.GroupReport
		NetBox  NetBoxView
		Metrics []string
	}{deployment(s.Status()), reports(s), s.NetBox(), driftMetrics(t, s)}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func reports(s *Service) []*drift.GroupReport {
	var out []*drift.GroupReport
	for _, g := range s.Groups() {
		out = append(out, g.Report)
	}
	return out
}

// driftGauges are the metrics that every replica sets from what it serves
// (ADR-0048).
var driftGauges = []string{
	"nbpdns_drift_last_refresh_timestamp_seconds", "nbpdns_drift_last_complete_refresh_timestamp_seconds",
	"nbpdns_drift_zones", "nbpdns_drift_rrset_changes", "nbpdns_drift_zone_drifted", "nbpdns_drift_problems",
	"nbpdns_drift_warnings", "nbpdns_netbox_up", "nbpdns_server_group_up", "nbpdns_server_group_last_success_timestamp_seconds",
}

// driftMetrics returns the series of s's drift gauges.
func driftMetrics(t *testing.T, s *Service) []string {
	t.Helper()
	var out []string
	for line := range strings.Lines(metricsText(t, s)) {
		name, _, _ := strings.Cut(strings.TrimSpace(line), "{")
		name, _, _ = strings.Cut(name, " ")
		if slices.Contains(driftGauges, name) {
			out = append(out, line)
		}
	}
	return out
}

// TestFollowerServesTheLeadersRefreshes runs refreshes on a leader, and
// checks after each that a follower on the same database, once it reloads,
// serves the same: the same status, reports, NetBox's records and drift
// metrics (ADR-0048).
func TestFollowerServesTheLeadersRefreshes(t *testing.T) {
	st := testStore(t)
	ctx := t.Context()
	groups := []Group{{Name: "site-a", URL: "https://a", Views: []string{"v"}}, {Name: "site-b", URL: "https://b", Views: []string{"v"}}}
	var r drift.Report
	var rerr error
	leader, _, _ := testService(t, Options{Groups: groups, History: st, Refresh: func(context.Context, []drift.ZoneRef) (drift.Report, error) {
		return r, rerr
	}})
	follower, _, _ := testService(t, Options{Groups: groups, History: st, Elected: true, Refresh: returning(report())})
	follower.Restore(ctx)
	if code, body := get(t, follower, http.MethodGet, "/readyz"); code != http.StatusServiceUnavailable || !strings.Contains(body, "follower") {
		t.Errorf("a follower on an empty database: /readyz = %d %q, want 503", code, body)
	}

	t0 := time.Date(2026, 11, 5, 1, 0, 0, 0, time.UTC)
	steps := []struct {
		name string
		run  func(at time.Time)
	}{
		{"a full refresh", func(at time.Time) {
			r = realistic(report(group("site-a", drift.StateDrift, drift.StateInSync), failedGroup("site-b")))
			r.NetBox = []dns.Zone{netboxZone("a.example.", "192.0.2.1"), netboxZone("b.example.", "192.0.2.2")}
			leader.refresh(ctx, at, batch{})
		}},
		{"a zone refresh", func(at time.Time) {
			// a.example. is back in sync, with a changed record.
			zg := drift.GroupReport{Group: "site-a", Status: drift.StatusOK, Compared: []string{"a.example."},
				Zones: []drift.ZoneReport{{Zone: "a.example.", State: drift.StateInSync}}}
			r = realistic(drift.Report{Complete: true, Groups: []drift.GroupReport{zg}})
			r.NetBox, r.Scope = []dns.Zone{netboxZone("a.example.", "192.0.2.10")}, &drift.Scope{Views: []string{"v"}, Names: []string{"a.example."}}
			leader.refreshZones(ctx, at, batch{events: 1, zones: map[drift.ZoneRef]bool{{View: "v", Name: "a.example."}: true}})
		}},
		{"NetBox can't be read", func(at time.Time) {
			r, rerr = drift.Report{}, errors.New("NetBox isn't reachable")
			leader.refresh(ctx, at, batch{})
			rerr = nil
		}},
		{"every group in sync, with a zone gone from NetBox", func(at time.Time) {
			r = realistic(report(group("site-a", drift.StateInSync), group("site-b", drift.StateInSync)))
			r.NetBox = []dns.Zone{netboxZone("a.example.", "192.0.2.10")}
			leader.refresh(ctx, at, batch{})
		}},
	}
	for i, step := range steps {
		step.run(t0.Add(time.Duration(i) * time.Minute))
		if leader.Version() != int64(i+1) {
			t.Errorf("%s: the leader's version = %d, want %d", step.name, leader.Version(), i+1)
		}
		if err := follower.Reload(ctx); err != nil {
			t.Fatalf("%s: Reload() = %v", step.name, err)
		}
		if want, got := servedBy(t, leader), servedBy(t, follower); got != want {
			t.Errorf("%s: the follower serves\n%s\nwant the leader's\n%s", step.name, got, want)
		}
		if follower.Version() != leader.Version() {
			t.Errorf("%s: the follower's version = %d, want the leader's, %d", step.name, follower.Version(), leader.Version())
		}
	}
	if code, _ := get(t, follower, http.MethodGet, "/readyz"); code != http.StatusOK {
		t.Errorf("a follower that has loaded the reports: /readyz = %d, want 200", code)
	}
	// The leader serves what it recorded, so it has nothing to reload.
	if err := leader.Reload(ctx); err != nil || leader.Version() != 4 {
		t.Errorf("the leader's Reload() = %v, version %d, want nothing new", err, leader.Version())
	}
}

// TestUnwrittenRefreshIsServedByNoReplica fails a refresh's write, and
// checks that neither the leader nor a follower serves it, and that the
// next refresh that's written is served by both (ADR-0048).
func TestUnwrittenRefreshIsServedByNoReplica(t *testing.T) {
	h := &flakyHistory{Store: testStore(t)}
	ctx := t.Context()
	var r drift.Report
	leader, _, _ := testService(t, Options{Groups: twoGroups, History: h, Refresh: func(context.Context, []drift.ZoneRef) (drift.Report, error) {
		return r, nil
	}})
	follower, _, _ := testService(t, Options{Groups: twoGroups, History: h, Elected: true, Refresh: returning(report())})
	t0 := time.Date(2026, 11, 5, 1, 0, 0, 0, time.UTC)

	r = realistic(report(group("site-a", drift.StateInSync), group("site-b", drift.StateInSync)))
	leader.refresh(ctx, t0, batch{})
	before := servedBy(t, leader)

	h.failing.Store(true)
	r = realistic(report(group("site-a", drift.StateDrift), group("site-b", drift.StateMissing)))
	leader.refresh(ctx, t0.Add(time.Minute), batch{})
	h.failing.Store(false)
	if got := servedBy(t, leader); got != before {
		t.Errorf("the leader serves the refresh it couldn't write:\n%s\nwant\n%s", got, before)
	}
	if err := follower.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if got := servedBy(t, follower); got != before {
		t.Errorf("the follower serves\n%s\nwant the last refresh written\n%s", got, before)
	}
	if st := leader.Status(); st.Database.HistoryWriteFailures != 1 || st.Schedule.Refreshes.Complete != 2 {
		t.Errorf("the leader's database %+v, refreshes %+v, want one write failure, and two refreshes run", st.Database, st.Schedule.Refreshes)
	}

	leader.refresh(ctx, t0.Add(2*time.Minute), batch{})
	if err := follower.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if got, want := servedBy(t, follower), servedBy(t, leader); got != want || got == before {
		t.Errorf("after the next refresh, the follower serves\n%s\nwant the leader's new one\n%s", got, want)
	}
}

// TestRestartServesNetBoxsRecords checks that a service that starts on a
// database that a refresh wrote serves NetBox's records at once (ADR-0048).
func TestRestartServesNetBoxsRecords(t *testing.T) {
	st := testStore(t)
	ctx := t.Context()
	r := realistic(report(group("site-a", drift.StateInSync)))
	r.NetBox = []dns.Zone{netboxZone("a.example.", "192.0.2.1")}
	first, _, _ := testService(t, Options{Groups: twoGroups, History: st, Refresh: returning(r)})
	t0 := time.Date(2026, 11, 5, 1, 0, 0, 0, time.UTC)
	first.refresh(ctx, t0, batch{})

	s, _, _ := testService(t, Options{Groups: twoGroups, History: st, Refresh: returning(report())})
	s.Restore(ctx)
	nb := s.NetBox()
	z, ok := nb.In([]string{"v"})("a.example.")
	if want := first.NetBox().AsOf; !nb.AsOf.Equal(want) || !ok || len(z.RRsets) != 1 || z.RRsets[0].Records[0].Value != "192.0.2.1" {
		t.Errorf("after a restart, NetBox's zones = %+v, want a.example.'s records as of %v", nb, want)
	}
	if st := s.Status(); st.NetBox.Up == nil || !*st.NetBox.Up {
		t.Errorf("after a restart, NetBox = %+v, want up", st.NetBox)
	}
}

// realistic returns r with each group's slices set, as drift.Compare sets
// them.
func realistic(r drift.Report) drift.Report {
	for i := range r.Groups {
		g := &r.Groups[i]
		if g.Status != drift.StatusOK {
			continue
		}
		if g.Zones == nil {
			g.Zones = []drift.ZoneReport{}
		}
		if g.Unmanaged == nil {
			g.Unmanaged = []string{}
		}
		if g.Problems == nil {
			g.Problems = []dns.Problem{}
		}
		if g.Warnings == nil {
			g.Warnings = []drift.Warning{}
		}
	}
	return r
}

// netboxZone returns NetBox's active zone name in view v, with an A record
// of value.
func netboxZone(name, value string) dns.Zone {
	return dns.Zone{Name: name, View: "v", Status: "active", Active: true, SOASerial: 1, Nameservers: []string{"ns1.example."},
		RRsets: []dns.RRset{{Name: "www." + name, Type: "A", TTL: 300, Records: []dns.Record{{Value: value, TTL: 300, Status: "active", Active: true}}}}}
}

// metricsText returns the service's metrics, as /metrics serves them.
func metricsText(t *testing.T, s *Service) string {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", http.NoBody))
	return rec.Body.String()
}

// TestSetTiming shortens the interval of a running service, which then
// refreshes on the new schedule, without waiting out the old one.
func TestSetTiming(t *testing.T) {
	refreshed := make(chan struct{}, 10)
	s, _, _ := testService(t, Options{Interval: time.Hour, Refresh: func(context.Context, []drift.ZoneRef) (drift.Report, error) {
		refreshed <- struct{}{}
		return report(), nil
	}})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go s.Run(ctx)
	<-refreshed
	s.SetTiming(Timing{Interval: 50 * time.Millisecond, Timeout: time.Second, WebhookDelay: 200 * time.Millisecond})
	select {
	case <-refreshed:
	case <-time.After(5 * time.Second):
		t.Fatal("no refresh after the interval was shortened from an hour")
	}
	status := s.Status()
	if status.Schedule.IntervalSeconds != 0.05 || status.Schedule.TimeoutSeconds != 1 || status.Webhooks.DelaySeconds != 0.2 {
		t.Errorf("schedule %+v, webhooks' delay %v, want the new timing", status.Schedule, status.Webhooks.DelaySeconds)
	}
}

// TestStatusDatabase checks how /status shows each database (ADR-0046).
func TestStatusDatabase(t *testing.T) {
	yes, no := true, false
	tests := []struct {
		name string
		db   DatabaseInfo
		want string
	}{
		{"SQLite", DatabaseInfo{Backend: "sqlite", URL: "sqlite:///var/lib/nbpdns/nbpdns.db", Path: "/var/lib/nbpdns/nbpdns.db"},
			"Database: /var/lib/nbpdns/nbpdns.db\n"},
		{"PostgreSQL over TLS", DatabaseInfo{Backend: "postgresql", URL: "postgres://nbpdns:xxxxx@db/nbpdns", TLS: &yes},
			"Database: PostgreSQL, postgres://nbpdns:xxxxx@db/nbpdns, over TLS\n"},
		{"PostgreSQL unencrypted", DatabaseInfo{Backend: "postgresql", URL: "postgres://db/nbpdns", TLS: &no},
			"Database: PostgreSQL, postgres://db/nbpdns, unencrypted\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _, _ := testService(t, Options{Groups: twoGroups, Database: tt.db, Refresh: returning(report())})
			status := s.Status()
			var page strings.Builder
			if err := writeStatus(&page, status); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(page.String(), tt.want) {
				t.Errorf("the status page doesn't have %q:\n%s", tt.want, page.String())
			}
			b, err := json.Marshal(status.Database)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatal(err)
			}
			if got["backend"] != tt.db.Backend || got["url"] != tt.db.URL || (got["tls"] == nil) != (tt.db.TLS == nil) {
				t.Errorf("database as JSON = %s, want %+v", b, tt.db)
			}
		})
	}
}
