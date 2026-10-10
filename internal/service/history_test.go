package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
)

// testStore returns a new, migrated database in a test's directory.
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
	s, logs, _ := testService(t, Options{Groups: twoGroups, History: st, DatabasePath: st.Path(), Refresh: func(context.Context, []drift.ZoneRef) (drift.Report, error) {
		return r, nil
	}})
	ctx := t.Context()
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	s.refresh(ctx, t0, batch{})
	r = report(group("site-a", drift.StateDrift), group("site-b", drift.StateInSync))
	s.refresh(ctx, t0.Add(time.Minute), batch{})

	got, err := st.RestoreDrift(ctx)
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
	s, logs, _ := testService(t, Options{Groups: groups, History: st, DatabasePath: st.Path(), Refresh: returning(report())})
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
	if lr := status.Schedule.LastRefresh; lr == nil || !lr.Finished.Equal(t0) || lr.Outcome != "incomplete" {
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
	if !strings.Contains(logs.String(), `"msg":"restored the last drift reports, so the service is ready; its first refresh starts now"`) {
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

func (failingHistory) RecordRefresh(context.Context, store.RefreshRecord) (int, []audit.Event, error) {
	return 0, nil, errors.New("disk I/O error")
}

func (failingHistory) RestoreDrift(context.Context) (store.Restored, error) {
	return store.Restored{}, errors.New("disk I/O error")
}

// TestHistoryWriteFailure checks that a refresh whose history can't be
// written is still served, and the failure counted, logged and shown.
func TestHistoryWriteFailure(t *testing.T) {
	s, logs, _ := testService(t, Options{Groups: twoGroups, History: failingHistory{}, DatabasePath: "/x/nbpdns.db",
		Refresh: returning(report(group("site-a", drift.StateDrift), group("site-b", drift.StateInSync)))})
	s.Restore(t.Context())
	s.refresh(t.Context(), time.Now(), batch{})
	status := s.Status()
	if !status.Ready || status.Groups[0].Counts.Drift != 1 {
		t.Errorf("status = %+v, want the refresh served", status)
	}
	d := status.Database
	if d.HistoryWriteFailures != 1 || d.LastWriteFailure == nil || !strings.Contains(d.LastWriteError, "disk I/O error") {
		t.Errorf("database = %+v, want one failure, with its error", d)
	}
	if m := metricsText(t, s); !strings.Contains(m, `nbpdns_db_write_failures_total{op="history"} 1`) {
		t.Errorf("the metrics don't count the failure:\n%s", m)
	}
	for _, want := range []string{"couldn't restore the last drift reports", "couldn't write the refresh to the drift history"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("no %q in the logs:\n%s", want, logs)
		}
	}
	var page strings.Builder
	if err := writeStatus(&page, status); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page.String(), "write failures: 1 refreshes couldn't be written to the drift history") {
		t.Errorf("the status page doesn't show the failure:\n%s", page.String())
	}
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
