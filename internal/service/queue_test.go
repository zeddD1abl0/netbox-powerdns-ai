package service

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
)

// webhooksOf returns s's webhooks' state as /status shows it, as JSON.
func webhooksOf(t *testing.T, s *Service) string {
	t.Helper()
	b, err := json.Marshal(s.Status().Webhooks)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// queuedIn returns the rows that st's queue holds, and its mark.
func queuedIn(t *testing.T, st *store.Store) ([]store.QueuedRefresh, int64) {
	t.Helper()
	w, err := st.LoadWebhooks(t.Context(), -1, 0)
	if err != nil {
		t.Fatal(err)
	}
	return w.Queued, w.Covered
}

// TestWebhookToAFollower sends a webhook to a follower, and checks that the
// leader refreshes its zone once the delay has passed since it, that both
// show the same webhooks' state throughout, and that the refresh, once
// recorded, empties the queue (ADR-0048).
func TestWebhookToAFollower(t *testing.T) {
	st := testStore(t)
	f := newFakeDrift()
	leader, _, _ := testService(t, Options{Groups: served, History: st, Refresh: f.refresh, Webhooks: true, WebhookDelay: 50 * time.Millisecond})
	follower, _, _ := testService(t, Options{Groups: served, History: st, Elected: true, Refresh: f.refresh, Webhooks: true,
		WebhookDelay: 50 * time.Millisecond})
	running(t, leader, f)

	// The delay runs from the webhook's arrival, before its write.
	sent := time.Now()
	if !notify(t, follower, event("r1", "alice"), zonesOf("a.example.")) {
		t.Fatal("the follower didn't queue the zone")
	}
	p := follower.Status().Webhooks.Pending
	if p.Events != 1 || !slices.Equal(p.Zones, []string{"v/a.example."}) || p.Due == nil {
		t.Errorf("the follower's pending %+v", p)
	}
	// The change feed finds the follower's webhook in the database.
	if err := leader.ReloadWebhooks(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got, want := webhooksOf(t, leader), webhooksOf(t, follower); got != want {
		t.Errorf("the leader shows\n%s\nwant the follower's\n%s", got, want)
	}
	if got := testutil.ToFloat64(leader.o.Metrics.PendingZones); got != 1 {
		t.Errorf("the leader's pending zones: %v", got)
	}
	if z := f.next(t); !reflect.DeepEqual(z, []drift.ZoneRef{{View: "v", Name: "a.example."}}) {
		t.Fatalf("the leader refreshed %v", z)
	}
	if took := time.Since(sent); took < 45*time.Millisecond {
		t.Errorf("the zone refresh came %v after the webhook, before its delay", took)
	}
	waitFor(t, func() bool { return leader.Status().Webhooks.LastRefresh != nil })
	if q, covered := queuedIn(t, st); q != nil || covered != 1 {
		t.Errorf("once the refresh is recorded, the queue holds %+v, covered through %d", q, covered)
	}
	if err := follower.ReloadWebhooks(t.Context()); err != nil {
		t.Fatal(err)
	}
	got, want := webhooksOf(t, follower), webhooksOf(t, leader)
	if got != want {
		t.Errorf("the follower shows\n%s\nwant the leader's\n%s", got, want)
	}
	w := follower.Status().Webhooks
	if w.Pending.Events != 0 || w.LastRefresh.Events != 1 || !slices.Equal(w.LastRefresh.Zones, []string{"v/a.example."}) ||
		w.LastEvent == nil || w.LastEvent.Request != (Request{"r1", "alice"}) || testutil.ToFloat64(follower.o.Metrics.PendingZones) != 0 {
		t.Errorf("the follower's webhooks %s", got)
	}
	f.none(t, 100*time.Millisecond)
}

// TestQueuedWebhookSurvivesARestart queues a webhook, and checks that a
// service that starts on the database shows it, and refreshes it.
func TestQueuedWebhookSurvivesARestart(t *testing.T) {
	st := testStore(t)
	first, _, _ := testService(t, Options{Groups: served, History: st, Webhooks: true})
	notify(t, first, event("r1", "alice"), zonesOf("a.example."))

	f := newFakeDrift()
	s, _, _ := testService(t, Options{Groups: served, History: st, Refresh: f.refresh, Webhooks: true})
	s.Restore(t.Context())
	if got, want := webhooksOf(t, s), webhooksOf(t, first); got != want {
		t.Errorf("after the restart, the webhooks are\n%s\nwant\n%s", got, want)
	}
	// The first refresh is a full one, which covers the queue.
	running(t, s, f)
	waitFor(t, func() bool { return s.Status().Webhooks.LastRefresh != nil })
	if r := s.Status().Webhooks.LastRefresh; !r.Full || r.Reason != "the scheduled refresh came first" || r.Events != 1 {
		t.Errorf("last refresh %+v", r)
	}
	if q, covered := queuedIn(t, st); q != nil || covered != 1 {
		t.Errorf("the queue holds %+v, covered through %d", q, covered)
	}
}

// TestQueuedWebhookSurvivesATakeover stops a leader during the zone
// refresh of a webhook, and checks that its refresh, which isn't recorded,
// leaves the webhook queued, and that the next leader refreshes it.
func TestQueuedWebhookSurvivesATakeover(t *testing.T) {
	st := testStore(t)
	started := make(chan struct{})
	old, _, _ := testService(t, Options{Groups: served, History: st, WebhookDelay: 10 * time.Millisecond,
		Refresh: func(ctx context.Context, zones []drift.ZoneRef) (drift.Report, error) {
			if zones == nil {
				return report(group("site-a", drift.StateInSync)), nil
			}
			close(started)
			<-ctx.Done()
			return drift.Report{}, ctx.Err()
		}})
	ctx, stop := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		old.Run(ctx)
		close(done)
	}()
	waitFor(t, func() bool { return old.Status().Schedule.LastRefresh != nil })
	notify(t, old, event("r1", "alice"), zonesOf("a.example."))
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("no zone refresh")
	}
	// The leader steps down, or stops, during the refresh.
	stop()
	<-done
	if q, _ := queuedIn(t, st); len(q) != 1 {
		t.Fatalf("after the leader stopped, the queue holds %+v", q)
	}

	f := newFakeDrift()
	next, _, _ := testService(t, Options{Groups: served, History: st, Refresh: f.refresh})
	next.Restore(t.Context())
	running(t, next, f)
	waitFor(t, func() bool { return next.Status().Webhooks.LastRefresh != nil })
	if r := next.Status().Webhooks.LastRefresh; !r.Full || r.Events != 1 || !reflect.DeepEqual(r.Requests, []Request{{"r1", "alice"}}) {
		t.Errorf("the new leader's last refresh %+v", r)
	}
	if q, covered := queuedIn(t, st); q != nil || covered != 1 {
		t.Errorf("the queue holds %+v, covered through %d", q, covered)
	}
}

// TestUnwrittenRefreshLeavesTheWebhooksQueued fails the write of a zone
// refresh, and checks that its webhook stays queued, on every replica, and
// that the next refresh, once written, covers it (ADR-0048).
func TestUnwrittenRefreshLeavesTheWebhooksQueued(t *testing.T) {
	h := &flakyHistory{Store: testStore(t)}
	ctx := t.Context()
	f := newFakeDrift()
	leader, _, _ := testService(t, Options{Groups: served, History: h, Refresh: f.refresh})
	follower, _, _ := testService(t, Options{Groups: served, History: h, Elected: true, Refresh: f.refresh})
	t0 := time.Date(2026, 11, 6, 1, 0, 0, 0, time.UTC)
	leader.refresh(ctx, t0, batch{})
	notify(t, follower, event("r1", "alice"), zonesOf("a.example."))
	if err := leader.ReloadWebhooks(ctx); err != nil {
		t.Fatal(err)
	}

	b, full := leader.take(false)
	h.failing.Store(true)
	leader.refreshZones(ctx, t0.Add(time.Minute), b)
	h.failing.Store(false)
	if full || leader.Status().Database.HistoryWriteFailures != 1 {
		t.Fatalf("full %t, database %+v", full, leader.Status().Database)
	}
	if q, _ := queuedIn(t, h.Store); len(q) != 1 {
		t.Errorf("after the failed write, the queue holds %+v", q)
	}
	if err := follower.ReloadWebhooks(ctx); err != nil {
		t.Fatal(err)
	}
	for name, s := range map[string]*Service{"leader": leader, "follower": follower} {
		if w := s.Status().Webhooks; w.Pending.Events != 1 || w.LastRefresh != nil {
			t.Errorf("the %s's webhooks after the failed write: %+v", name, w)
		}
	}
	// The next refresh, here the scheduled one, covers it.
	b, full = leader.take(true)
	if !full || b.events != 1 {
		t.Fatalf("the scheduled refresh took %+v, full %t", b, full)
	}
	leader.refresh(ctx, t0.Add(2*time.Minute), b)
	if q, covered := queuedIn(t, h.Store); q != nil || covered != 1 {
		t.Errorf("after the full refresh, the queue holds %+v, covered through %d", q, covered)
	}
	if err := follower.ReloadWebhooks(ctx); err != nil {
		t.Fatal(err)
	}
	if got, want := webhooksOf(t, follower), webhooksOf(t, leader); got != want {
		t.Errorf("the follower shows\n%s\nwant the leader's\n%s", got, want)
	}
}

// TestWebhookThatCantBeWritten checks that Notify reports a webhook that
// the database can't take, and counts the failure.
func TestWebhookThatCantBeWritten(t *testing.T) {
	s, _, _ := testService(t, Options{Groups: served, History: failingHistory{}})
	if queued, err := s.Notify(t.Context(), event("r1", ""), zonesOf("a.example.")); queued || err == nil {
		t.Errorf("Notify() = %t, %v, want an error", queued, err)
	}
	if m := metricsText(t, s); !strings.Contains(m, `nbpdns_db_write_failures_total{op="webhook"} 1`) {
		t.Errorf("the webhook's write failure isn't counted:\n%s", m)
	}
}
