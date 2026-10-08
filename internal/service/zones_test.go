package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/metrics"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/webhook"
)

// served are the groups of the zone refresh tests: site-a serves view v.
var served = []Group{{Name: "site-a", Views: []string{"v"}}}

// event returns a NetBox event from the request id, by user.
func event(id, user string) webhook.Event {
	return webhook.Event{Event: "updated", ObjectType: webhook.TypeRecord, Request: &webhook.Request{ID: id, User: user}}
}

func zonesOf(names ...string) webhook.Refresh {
	r := webhook.Refresh{}
	for _, n := range names {
		r.Zones = append(r.Zones, webhook.Zone{View: "v", Name: n})
	}
	return r
}

func TestNotify(t *testing.T) {
	s, _, _ := testService(t, Options{Groups: served})
	if s.Notify(t.Context(), event("r1", "alice"), webhook.Refresh{Reason: "nbpdns doesn't read extras.tag objects"}) {
		t.Error("an ignored event was queued")
	}
	if s.Notify(t.Context(), event("r1", "alice"), webhook.Refresh{Zones: []webhook.Zone{{View: "other", Name: "a.example."}}}) {
		t.Error("a zone in a view that no group serves was queued")
	}
	if !s.Notify(t.Context(), event("r1", "alice"), zonesOf("a.example.", "b.example.")) ||
		!s.Notify(t.Context(), event("r1", "alice"), zonesOf("a.example.")) ||
		!s.Notify(t.Context(), event("r2", "bob"), zonesOf("c.example.")) {
		t.Fatal("served zones weren't queued")
	}
	if got := testutil.ToFloat64(s.o.Metrics.PendingZones); got != 3 {
		t.Errorf("%v pending zones, want 3", got)
	}
	b := s.take()
	if b.events != 3 || b.full || !slices.Equal(b.names(), []string{"v/a.example.", "v/b.example.", "v/c.example."}) ||
		!reflect.DeepEqual(b.requests, []Request{{"r1", "alice"}, {"r2", "bob"}}) {
		t.Errorf("batch %+v", b)
	}
	if got := testutil.ToFloat64(s.o.Metrics.PendingZones); got != 0 || s.st.pending.events != 0 {
		t.Errorf("after take, %v pending zones, %d events", got, s.st.pending.events)
	}
	if e := s.st.lastEvent; e == nil || e.Request != (Request{"r2", "bob"}) || e.ObjectType != webhook.TypeRecord {
		t.Errorf("last event %+v", e)
	}

	t.Run("a full refresh", func(t *testing.T) {
		s.Notify(t.Context(), event("r3", ""), zonesOf("a.example."))
		s.Notify(t.Context(), event("r3", ""), webhook.Refresh{Full: true, Reason: "a view was updated"})
		if b := s.take(); !b.full || b.reason != "a view was updated" {
			t.Errorf("batch %+v", b)
		}
	})
	t.Run("too many zones", func(t *testing.T) {
		for i := range maxZones + 5 {
			s.Notify(t.Context(), event("r4", ""), zonesOf(fmt.Sprintf("z%d.example.", i)))
		}
		if b := s.take(); !b.full || b.reason != "more than 100 zones changed" || len(b.zones) != maxZones+1 || len(b.requests) != 1 {
			t.Errorf("full %t for %q, with %d zones and %d requests", b.full, b.reason, len(b.zones), len(b.requests))
		}
	})
}

// fakeDrift is a RefreshFunc that records each call, whose zones are nil
// for a full refresh, and answers as every zone being in sync.
type fakeDrift struct {
	calls   chan []drift.ZoneRef
	delay   time.Duration
	fail    error
	running atomic.Int32
	most    atomic.Int32
}

func newFakeDrift() *fakeDrift { return &fakeDrift{calls: make(chan []drift.ZoneRef, 100)} }

func (f *fakeDrift) refresh(_ context.Context, zones []drift.ZoneRef) (drift.Report, error) {
	if n := f.running.Add(1); n > f.most.Load() {
		f.most.Store(n)
	}
	defer f.running.Add(-1)
	time.Sleep(f.delay)
	f.calls <- zones
	if f.fail != nil {
		return drift.Report{}, f.fail
	}
	if zones == nil {
		return report(group("site-a", drift.StateInSync, drift.StateInSync)), nil
	}
	g := drift.GroupReport{Group: "site-a", Status: drift.StatusOK}
	for _, z := range zones {
		g.Compared = append(g.Compared, z.Name)
		g.Zones = append(g.Zones, drift.ZoneReport{Zone: z.Name, State: drift.StateInSync})
	}
	return drift.Report{Complete: true, Groups: []drift.GroupReport{g}, Scope: &drift.Scope{}}, nil
}

// next returns the zones of the next call, or fails t if none comes
// within a second.
func (f *fakeDrift) next(t *testing.T) []drift.ZoneRef {
	t.Helper()
	select {
	case z := <-f.calls:
		return z
	case <-time.After(time.Second):
		t.Fatal("no refresh")
		return nil
	}
}

// none fails t if a refresh comes within d.
func (f *fakeDrift) none(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case z := <-f.calls:
		t.Fatalf("an unexpected refresh of %v", z)
	case <-time.After(d):
	}
}

// running starts s, and waits for its first, full, refresh.
func running(t *testing.T, s *Service, f *fakeDrift) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	if z := f.next(t); z != nil {
		t.Fatalf("the first refresh was of %v, not of every zone", z)
	}
}

func TestZoneRefreshAfterTheQuietSpell(t *testing.T) {
	f := newFakeDrift()
	s, _, _ := testService(t, Options{Groups: served, Refresh: f.refresh, WebhookDelay: 50 * time.Millisecond})
	running(t, s, f)
	start := time.Now()
	for i, n := range []string{"a.example.", "b.example.", "a.example."} {
		if i > 0 {
			time.Sleep(20 * time.Millisecond)
		}
		s.Notify(t.Context(), event("r1", "alice"), zonesOf(n))
	}
	last := time.Now()
	z := f.next(t)
	if took := time.Since(last); took < 40*time.Millisecond {
		t.Errorf("the zone refresh came %v after the last webhook, before the quiet spell", took)
	}
	if want := []drift.ZoneRef{{View: "v", Name: "a.example."}, {View: "v", Name: "b.example."}}; !reflect.DeepEqual(z, want) {
		t.Errorf("refreshed %v, want %v, %v after the first webhook", z, want, time.Since(start))
	}
	f.none(t, 100*time.Millisecond)
	if got := testutil.ToFloat64(s.o.Metrics.ZoneRefreshes.WithLabelValues(metrics.OutcomeComplete)); got != 1 {
		t.Errorf("%v zone refreshes counted", got)
	}
	s.mu.Lock()
	w := s.st.lastWebhookRefresh
	s.mu.Unlock()
	if w == nil || w.Full || !slices.Equal(w.Zones, []string{"v/a.example.", "v/b.example."}) || w.Outcome != metrics.OutcomeComplete ||
		w.Events != 3 || !reflect.DeepEqual(w.Requests, []Request{{"r1", "alice"}}) {
		t.Errorf("last webhook refresh %+v", w)
	}
}

func TestZoneRefreshWaitsNoLongerThanTheCap(t *testing.T) {
	f := newFakeDrift()
	s, _, _ := testService(t, Options{Groups: served, Refresh: f.refresh, WebhookDelay: 60 * time.Millisecond, maxWait: 150 * time.Millisecond})
	running(t, s, f)
	first := time.Now()
	stop := make(chan struct{})
	go func() {
		for {
			s.Notify(context.Background(), event("r1", ""), zonesOf("a.example."))
			select {
			case <-stop:
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	}()
	defer close(stop)
	f.next(t)
	// Webhooks keep coming, so without the cap it would never come. The
	// upper bound allows for a slow machine.
	if took := time.Since(first); took < 140*time.Millisecond || took > 2*time.Second {
		t.Errorf("the zone refresh came %v after the first webhook, though webhooks kept coming; want about 150ms", took)
	}
}

func TestFullRefreshFromAWebhook(t *testing.T) {
	f := newFakeDrift()
	s, _, _ := testService(t, Options{Groups: served, Refresh: f.refresh, WebhookDelay: 20 * time.Millisecond})
	running(t, s, f)
	s.Notify(t.Context(), event("r1", ""), zonesOf("a.example."))
	s.Notify(t.Context(), event("r1", ""), webhook.Refresh{Full: true, Reason: "a view was updated"})
	before := time.Now()
	if z := f.next(t); z != nil {
		t.Fatalf("refreshed %v, want every zone", z)
	}
	f.none(t, 100*time.Millisecond)
	s.mu.Lock()
	next, w := s.st.next, s.st.lastWebhookRefresh
	s.mu.Unlock()
	// The schedule starts again from the full refresh.
	if next.Before(before.Add(time.Hour - time.Second)) {
		t.Errorf("the next scheduled refresh is at %v", next)
	}
	if w == nil || !w.Full || w.Reason != "a view was updated" || w.Zones != nil {
		t.Errorf("last webhook refresh %+v", w)
	}
}

func TestScheduledRefreshCoversTheZonesWaiting(t *testing.T) {
	f := newFakeDrift()
	s, _, _ := testService(t, Options{Groups: served, Refresh: f.refresh, Interval: 150 * time.Millisecond, WebhookDelay: 2 * time.Second})
	running(t, s, f)
	s.Notify(t.Context(), event("r1", ""), zonesOf("a.example."))
	if z := f.next(t); z != nil {
		t.Fatalf("refreshed %v, want the scheduled full refresh", z)
	}
	// Nothing is left for a zone refresh.
	s.mu.Lock()
	events := s.st.pending.events
	s.mu.Unlock()
	if events != 0 {
		t.Errorf("%d events still waiting", events)
	}
	waitFor(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.st.lastWebhookRefresh != nil
	})
	if w := s.Status().Webhooks.LastRefresh; !w.Full || w.Reason != "the scheduled refresh came first" || w.Zones != nil {
		t.Errorf("last webhook refresh %+v", w)
	}
}

func TestRefreshesNeverOverlap(t *testing.T) {
	f := newFakeDrift()
	f.delay = 40 * time.Millisecond
	s, _, _ := testService(t, Options{Groups: served, Refresh: f.refresh, Interval: 100 * time.Millisecond, WebhookDelay: 10 * time.Millisecond})
	running(t, s, f)
	deadline := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) {
		s.Notify(t.Context(), event("r1", ""), zonesOf("a.example."))
		time.Sleep(15 * time.Millisecond)
	}
	if most := f.most.Load(); most != 1 {
		t.Errorf("%d refreshes at once", most)
	}
}

func TestZoneRefreshIsATraceLinkedToItsWebhooks(t *testing.T) {
	f := newFakeDrift()
	s, logs, spans := testService(t, Options{Groups: served, Refresh: f.refresh, WebhookDelay: 20 * time.Millisecond})
	running(t, s, f)
	var hooks []trace.SpanContext
	for i, id := range []string{"r1", "r2"} {
		ctx, span := s.o.Tracer.Start(t.Context(), "POST /api/netbox-events")
		hooks = append(hooks, span.SpanContext())
		s.Notify(ctx, event(id, []string{"alice", "bob"}[i]), zonesOf("a.example."))
		span.End()
	}
	f.next(t)
	var zr sdktrace.ReadOnlySpan
	waitFor(t, func() bool {
		for _, sp := range spans.Ended() {
			if sp.Name() == "drift zone refresh" {
				zr = sp
			}
		}
		return zr != nil
	})
	var linked []trace.SpanContext
	for _, l := range zr.Links() {
		linked = append(linked, l.SpanContext)
	}
	if !reflect.DeepEqual(linked, hooks) || zr.Parent().IsValid() {
		t.Errorf("links %v, want the webhooks' %v; parent %v", linked, hooks, zr.Parent())
	}
	attrs := map[attribute.Key]string{}
	for _, a := range zr.Attributes() {
		attrs[a.Key] = a.Value.String()
	}
	if attrs["netbox.request_ids"] != `["r1","r2"]` || attrs["netbox.users"] != `["alice","bob"]` || attrs["nbpdns.zones"] != `["v/a.example."]` {
		t.Errorf("attributes %v", attrs)
	}
	waitFor(t, func() bool { return strings.Contains(logs.String(), `"msg":"zones refreshed"`) })
	for line := range strings.Lines(logs.String()) {
		if strings.Contains(line, `"msg":"zones refreshed"`) || strings.Contains(line, "refreshing the zones") {
			if !strings.Contains(line, `"netbox_request_ids":["r1","r2"]`) || !strings.Contains(line, `"trace_id":"`+zr.SpanContext().TraceID().String()) {
				t.Errorf("log line without the NetBox requests or the trace: %s", line)
			}
		}
	}
}

// waitFor waits up to a second for ok.
func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(time.Second); !ok(); {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRecordZones(t *testing.T) {
	groups := []Group{{Name: "site-a", Views: []string{"v"}}, {Name: "site-b", Views: []string{"v"}}, {Name: "site-c", Views: []string{"v"}}}
	s, logs, _ := testService(t, Options{Groups: groups})
	t0 := time.Date(2026, 10, 8, 1, 0, 0, 0, time.UTC)
	full := report(group("site-a", drift.StateInSync, drift.StateDrift, drift.StateInSync), group("site-b", drift.StateInSync), failedGroup("site-c"))
	full.NetBox = []dns.Zone{{Name: "a.example.", View: "v", Active: true}, {Name: "b.example.", View: "v", Active: true}}
	s.record(t.Context(), s.o.Log, full, nil, t0, t0)
	apiReport, _ := s.Group("site-a")
	apiNetBox := s.NetBox()

	// b.example. is back in sync, and c.example. was deleted from NetBox,
	// which the primary still serves.
	t1 := t0.Add(time.Minute)
	zr := drift.Report{Complete: false, Groups: []drift.GroupReport{
		{Group: "site-a", Status: drift.StatusOK, Compared: []string{"b.example.", "c.example."},
			Zones: []drift.ZoneReport{{Zone: "b.example.", State: drift.StateInSync}}, Unmanaged: []string{"c.example."}},
		failedGroup("site-b"),
		// site-c has no report to merge into.
		{Group: "site-c", Status: drift.StatusOK, Compared: []string{"b.example."}},
	},
		NetBox: []dns.Zone{{Name: "b.example.", View: "v", Active: true, SOASerial: 2}},
		Scope:  &drift.Scope{Views: []string{"v"}, Names: []string{"b.example.", "c.example."}},
	}
	if got := s.recordZones(t.Context(), s.o.Log, zr, nil, t1, t1); got != metrics.OutcomeIncomplete {
		t.Errorf("outcome %s", got)
	}
	a, _ := s.Group("site-a")
	var states []string
	for _, z := range a.Report.Zones {
		states = append(states, z.Zone+"="+z.State)
	}
	// c.example. left the zones, for the unmanaged ones.
	if !slices.Equal(states, []string{"a.example.=in_sync", "b.example.=in_sync"}) ||
		a.Report.Counts != (drift.Counts{InSync: 2, Unmanaged: 1}) || !slices.Equal(a.Report.Unmanaged, []string{"c.example."}) ||
		*a.Info.LastSuccess != t1 {
		t.Errorf("site-a: zones %v, counts %+v, unmanaged %v, last success %v", states, a.Report.Counts, a.Report.Unmanaged, a.Info.LastSuccess)
	}
	// The drifted zone's series is gone.
	if n := testutil.CollectAndCount(s.o.Metrics.ZoneDrifted); n != 0 {
		t.Errorf("%d drifted-zone series", n)
	}
	if got := testutil.ToFloat64(s.o.Metrics.Zones.WithLabelValues("site-a", drift.StateUnmanaged)); got != 1 {
		t.Errorf("site-a's unmanaged zones: %v", got)
	}
	if got := testutil.ToFloat64(s.o.Metrics.Zones.WithLabelValues("site-a", drift.StateInSync)); got != 2 {
		t.Errorf("site-a's in-sync zones: %v", got)
	}
	b, _ := s.Group("site-b")
	if b.Info.Status != drift.StatusFailed || b.Report == nil || len(b.Report.Zones) != 1 {
		t.Errorf("site-b %+v, report %+v", b.Info, b.Report)
	}
	if c, _ := s.Group("site-c"); c.Report != nil || c.Info.Status != drift.StatusFailed {
		t.Errorf("site-c %+v, report %+v", c.Info, c.Report)
	}
	nb := s.NetBox()
	if z := nb.Zones["v"]["b.example."]; z.SOASerial != 2 || len(nb.Zones["v"]) != 2 || nb.AsOf != t0 {
		t.Errorf("NetBox %+v", nb)
	}
	// What the API held is as it was.
	if apiReport.Report.Counts != (drift.Counts{InSync: 2, Drift: 1}) || len(apiReport.Report.Zones) != 3 ||
		apiNetBox.Zones["v"]["b.example."].SOASerial != 0 {
		t.Errorf("a report the API held changed: %+v, %+v", apiReport.Report.Counts, apiNetBox.Zones["v"])
	}
	if !strings.Contains(logs.String(), `"msg":"zones refreshed"`) {
		t.Errorf("no zones refreshed line:\n%s", logs)
	}

	t.Run("NetBox can't be read", func(t *testing.T) {
		s.recordZones(t.Context(), s.o.Log, drift.Report{}, errors.New("NetBox isn't reachable"), t1, t1)
		if st := s.Status(); *st.NetBox.Up || st.NetBox.Error != "NetBox isn't reachable" {
			t.Errorf("NetBox %+v", st.NetBox)
		}
		if a, _ := s.Group("site-a"); a.Report.Counts != (drift.Counts{InSync: 2, Unmanaged: 1}) {
			t.Errorf("site-a's report changed: %+v", a.Report.Counts)
		}
	})
	t.Run("out of time", func(t *testing.T) {
		s.recordZones(t.Context(), s.o.Log, drift.Report{}, &TimeoutError{Timeout: time.Minute}, t1, t1)
		if got := testutil.ToFloat64(s.o.Metrics.ZoneRefreshes.WithLabelValues(metrics.OutcomeFailed)); got != 2 {
			t.Errorf("%v failed zone refreshes", got)
		}
	})
}

func TestMergeNetBox(t *testing.T) {
	old := NetBoxView{AsOf: time.Unix(1, 0), Zones: map[string]map[string]dns.Zone{
		"v": {"a.example.": {Name: "a.example.", View: "v"}, "b.example.": {Name: "b.example.", View: "v"}},
		"w": {"b.example.": {Name: "b.example.", View: "w"}},
		"x": {"x.example.": {Name: "x.example.", View: "x"}},
	}}
	got := mergeNetBox(old, []dns.Zone{{Name: "b.example.", View: "v", SOASerial: 7}, {Name: "n.example.", View: "w"}},
		&drift.Scope{Views: []string{"v", "w"}, Names: []string{"b.example.", "n.example."}})
	want := map[string]map[string]dns.Zone{
		"v": {"a.example.": {Name: "a.example.", View: "v"}, "b.example.": {Name: "b.example.", View: "v", SOASerial: 7}},
		"w": {"n.example.": {Name: "n.example.", View: "w"}},
		"x": {"x.example.": {Name: "x.example.", View: "x"}},
	}
	if !reflect.DeepEqual(got.Zones, want) || got.AsOf != old.AsOf {
		t.Errorf("merged %+v", got)
	}
	if old.Zones["v"]["b.example."].SOASerial != 0 || len(old.Zones["w"]) != 1 {
		t.Errorf("the old view changed: %+v", old.Zones)
	}
	// A view left with no zones goes.
	if got := mergeNetBox(old, nil, &drift.Scope{Views: []string{"x"}, Names: []string{"x.example."}}); got.Zones["x"] != nil {
		t.Errorf("view x %+v", got.Zones["x"])
	}
}

func TestNotifyIsSafeWhileRefreshing(t *testing.T) {
	f := newFakeDrift()
	s, _, _ := testService(t, Options{Groups: served, Refresh: f.refresh, WebhookDelay: 5 * time.Millisecond})
	running(t, s, f)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			for j := range 20 {
				s.Notify(context.Background(), event("r", ""), zonesOf(fmt.Sprintf("z%d-%d.example.", i, j)))
				_ = s.Status()
				_ = s.Groups()
			}
		})
	}
	wg.Wait()
}

func TestStatusShowsTheWebhooks(t *testing.T) {
	text := func(s *Service) string {
		var b strings.Builder
		if err := writeStatus(&b, s.Status()); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	off, _, _ := testService(t, Options{Groups: served})
	if w := off.Status().Webhooks; w.Enabled || w.LastEvent != nil || w.Pending.Zones == nil || w.LastRefresh != nil {
		t.Errorf("webhooks off: %+v", w)
	}
	if !strings.Contains(text(off), "NetBox's webhooks are off; set netbox.webhook_secret") {
		t.Errorf("status page:\n%s", text(off))
	}

	s, _, _ := testService(t, Options{Groups: served, Webhooks: true})
	s.Notify(t.Context(), event("r1", "alice"), zonesOf("a.example."))
	w := s.Status().Webhooks
	if !w.Enabled || w.DelaySeconds != 3 || w.LastEvent == nil || w.LastEvent.Request != (Request{"r1", "alice"}) ||
		w.Pending.Events != 1 || !slices.Equal(w.Pending.Zones, []string{"v/a.example."}) || w.Pending.Due == nil {
		t.Errorf("webhooks %+v, last event %+v", w, w.LastEvent)
	}
	page := text(s)
	for _, want := range []string{
		"NetBox's webhooks, each zone refreshed once they stop for 3s:",
		"netbox_dns.record updated, request r1 by alice",
		"waiting:       v/a.example., due ",
		"last refresh:  none yet",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("no %q in the status page:\n%s", want, page)
		}
	}
	s.noteWebhookRefresh(s.take(), false, time.Now(), time.Now(), metrics.OutcomeComplete, nil)
	if page := text(s); !strings.Contains(page, "complete, of v/a.example.") || !strings.Contains(page, "waiting:       nothing") {
		t.Errorf("status page:\n%s", page)
	}
}
