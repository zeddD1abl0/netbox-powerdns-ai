package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/logging"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/metrics"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/tracing"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/version"
)

// syncBuffer is a bytes.Buffer that's safe for concurrent use.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// testService returns a service that refreshes with refresh, logging JSON
// to the returned buffer, and recording its spans.
func testService(t *testing.T, o Options) (*Service, *syncBuffer, *tracetest.SpanRecorder) {
	t.Helper()
	logs := &syncBuffer{}
	log, err := logging.New(logs, "json", "debug")
	if err != nil {
		t.Fatal(err)
	}
	spans := tracetest.NewSpanRecorder()
	o.Log = log
	o.Tracer = tracing.Tracer(tracing.NewProvider("test", sdktrace.WithSpanProcessor(spans)))
	o.Metrics = metrics.New(version.Info{Version: "test"})
	if o.Interval == 0 {
		o.Interval = time.Hour
	}
	if o.Timeout == 0 {
		o.Timeout = time.Minute
	}
	return New(o), logs, spans
}

// group returns a compared group's report, with zones in the given states.
func group(name string, states ...string) drift.GroupReport {
	g := drift.GroupReport{Group: name, Status: drift.StatusOK}
	for i, st := range states {
		z := drift.ZoneReport{Zone: string(rune('a'+i)) + ".example.", State: st}
		if st == drift.StateDrift {
			z.Changes = []drift.Change{{Name: "www." + z.Zone, Type: "A", Kind: drift.ChangeChanged}}
		}
		g.Zones = append(g.Zones, z)
		switch st {
		case drift.StateInSync:
			g.Counts.InSync++
		case drift.StateDrift:
			g.Counts.Drift++
		case drift.StateMissing:
			g.Counts.Missing++
		}
	}
	return g
}

func failedGroup(name string) drift.GroupReport {
	return drift.GroupReport{Group: name, Status: drift.StatusFailed, Error: "PowerDNS isn't reachable"}
}

func report(groups ...drift.GroupReport) drift.Report {
	r := drift.Report{Complete: true, Groups: groups}
	for _, g := range groups {
		r.Complete = r.Complete && g.Status == drift.StatusOK
	}
	return r
}

func TestSchedule(t *testing.T) {
	t.Run("on time", func(t *testing.T) {
		var n atomic.Int32
		s, _, _ := testService(t, Options{Interval: 20 * time.Millisecond, Refresh: func(context.Context) (drift.Report, error) {
			n.Add(1)
			return report(), nil
		}})
		ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
		defer cancel()
		s.Run(ctx)
		if got := n.Load(); got < 4 || got > 9 {
			t.Errorf("%d refreshes in 150ms at a 20ms interval", got)
		}
	})
	t.Run("overrunning", func(t *testing.T) {
		var inFlight, most atomic.Int32
		s, logs, _ := testService(t, Options{Interval: 10 * time.Millisecond, Refresh: func(context.Context) (drift.Report, error) {
			if n := inFlight.Add(1); n > most.Load() {
				most.Store(n)
			}
			time.Sleep(30 * time.Millisecond)
			inFlight.Add(-1)
			return report(), nil
		}})
		ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer cancel()
		s.Run(ctx)
		if most.Load() != 1 || !strings.Contains(logs.String(), "took longer than drift.interval") {
			t.Errorf("%d refreshes at once; logs:\n%s", most.Load(), logs)
		}
	})
}

func TestShutdown(t *testing.T) {
	started := make(chan struct{})
	s, _, _ := testService(t, Options{Refresh: func(ctx context.Context) (drift.Report, error) {
		close(started)
		<-ctx.Done()
		return drift.Report{}, ctx.Err()
	}})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run didn't return after the context was canceled")
	}
	// The refresh cut short isn't counted as a failure.
	if got := testutil.ToFloat64(s.o.Metrics.Refreshes.WithLabelValues(metrics.OutcomeFailed)); got != 0 {
		t.Errorf("%v failed refreshes", got)
	}
}

func TestTimeout(t *testing.T) {
	// A good refresh, then two that run out of time: one while NetBox is
	// read, one while a group is. Neither blames them.
	var n atomic.Int32
	s, logs, _ := testService(t, Options{Timeout: 20 * time.Millisecond, Groups: []Group{{Name: "a", URL: "https://a"}},
		Refresh: func(ctx context.Context) (drift.Report, error) {
			switch n.Add(1) {
			case 1:
				return report(group("a", drift.StateInSync)), nil
			case 2:
				<-ctx.Done()
				return drift.Report{}, ctx.Err()
			default:
				<-ctx.Done()
				return report(drift.GroupReport{Group: "a", Status: drift.StatusFailed, Error: ctx.Err().Error()}), nil
			}
		}})
	m := s.o.Metrics
	for range 3 {
		s.refresh(t.Context(), time.Now())
	}
	if testutil.ToFloat64(m.Refreshes.WithLabelValues(metrics.OutcomeFailed)) != 2 ||
		testutil.ToFloat64(m.NetBoxUp.WithLabelValues()) != 1 || testutil.ToFloat64(m.GroupUp.WithLabelValues("a")) != 1 {
		t.Errorf("after two timeouts:\n%s", gathered(t, m))
	}
	st := s.Status()
	if lr := st.Schedule.LastRefresh; lr == nil || lr.Outcome != metrics.OutcomeFailed || !strings.Contains(lr.Error, "longer than drift.timeout") ||
		st.Groups[0].Status != drift.StatusOK || st.NetBox.Error != "" {
		t.Errorf("status after two timeouts: %+v", st)
	}
	if !strings.Contains(logs.String(), "took longer than drift.timeout") {
		t.Errorf("the timeout wasn't logged:\n%s", logs)
	}
}

// get answers method path through s's handler.
func get(t *testing.T, s *Service, method, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), method, path, http.NoBody))
	b, _ := io.ReadAll(rec.Result().Body)
	return rec.Code, string(b)
}

func TestHealth(t *testing.T) {
	clock := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	now := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return clock
	}
	advance := func(d time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		clock = clock.Add(d)
	}
	s, _, _ := testService(t, Options{now: now, Interval: 5 * time.Minute, Timeout: 10 * time.Minute,
		Refresh: func(context.Context) (drift.Report, error) { return report(group("a", drift.StateInSync)), nil }})

	if code, body := get(t, s, http.MethodGet, "/readyz"); code != http.StatusServiceUnavailable || !strings.Contains(body, "first drift refresh") {
		t.Errorf("readyz before a refresh: %d %q", code, body)
	}
	if code, _ := get(t, s, http.MethodGet, "/livez"); code != http.StatusOK {
		t.Errorf("livez at start: %d", code)
	}
	s.refresh(t.Context(), now())
	if code, _ := get(t, s, http.MethodGet, "/readyz"); code != http.StatusOK {
		t.Errorf("readyz after a refresh: %d", code)
	}
	if code, _ := get(t, s, http.MethodHead, "/readyz"); code != http.StatusOK {
		t.Errorf("HEAD readyz: %d", code)
	}
	// Liveness allows the interval, the timeout and a minute.
	advance(16 * time.Minute)
	if code, _ := get(t, s, http.MethodGet, "/livez"); code != http.StatusOK {
		t.Errorf("livez 16 minutes after a refresh started: %d", code)
	}
	advance(time.Minute)
	if code, body := get(t, s, http.MethodGet, "/livez"); code != http.StatusServiceUnavailable || !strings.Contains(body, "stuck") {
		t.Errorf("livez 17 minutes after a refresh started: %d %q", code, body)
	}
	if code, _ := get(t, s, http.MethodPost, "/livez"); code != http.StatusMethodNotAllowed {
		t.Errorf("POST livez: %d", code)
	}
	if code, _ := get(t, s, http.MethodGet, "/nothing"); code != http.StatusNotFound {
		t.Errorf("GET /nothing: %d", code)
	}
	if code, body := get(t, s, http.MethodGet, "/metrics"); code != http.StatusOK || !strings.Contains(body, `nbpdns_drift_zones{group="a",state="in_sync"} 1`) {
		t.Errorf("metrics: %d\n%s", code, body)
	}
}

func TestLastKnownState(t *testing.T) {
	s, logs, _ := testService(t, Options{})
	m := s.o.Metrics
	zones := func(g, state string) float64 { return testutil.ToFloat64(m.Zones.WithLabelValues(g, state)) }
	up := func(g string) float64 { return testutil.ToFloat64(m.GroupUp.WithLabelValues(g)) }
	t0 := time.Now()

	// Both groups read; zone a in site-a drifted.
	s.record(t.Context(), report(group("site-a", drift.StateDrift, drift.StateInSync), group("site-b", drift.StateInSync)), nil, t0, t0)
	if zones("site-a", drift.StateDrift) != 1 || zones("site-a", drift.StateInSync) != 1 || up("site-a") != 1 ||
		testutil.ToFloat64(m.ZoneDrifted.WithLabelValues("site-a", "a.example.", drift.StateDrift)) != 1 ||
		testutil.ToFloat64(m.RRsetChanges.WithLabelValues("site-a", drift.ChangeChanged)) != 1 ||
		testutil.ToFloat64(m.NetBoxUp.WithLabelValues()) != 1 {
		t.Fatalf("after a complete refresh:\n%s", gathered(t, m))
	}

	// site-a's primary fails: it keeps its counts and its drifted zone.
	s.record(t.Context(), report(failedGroup("site-a"), group("site-b", drift.StateInSync)), nil, t0, t0)
	if up("site-a") != 0 || zones("site-a", drift.StateDrift) != 1 || testutil.CollectAndCount(m.ZoneDrifted) != 1 ||
		testutil.ToFloat64(m.Refreshes.WithLabelValues(metrics.OutcomeIncomplete)) != 1 {
		t.Errorf("after site-a failed:\n%s", gathered(t, m))
	}

	// NetBox fails: everything keeps its value, and the groups' up too.
	s.record(t.Context(), drift.Report{}, errors.New("NetBox isn't reachable"), t0, t0)
	if testutil.ToFloat64(m.NetBoxUp.WithLabelValues()) != 0 || up("site-a") != 0 || up("site-b") != 1 || zones("site-a", drift.StateDrift) != 1 ||
		testutil.ToFloat64(m.Refreshes.WithLabelValues(metrics.OutcomeFailed)) != 1 {
		t.Errorf("after NetBox failed:\n%s", gathered(t, m))
	}

	// site-a back, and in sync: its drifted zone's series goes.
	s.record(t.Context(), report(group("site-a", drift.StateInSync, drift.StateInSync), group("site-b", drift.StateMissing)), nil, t0, t0)
	if up("site-a") != 1 || zones("site-a", drift.StateDrift) != 0 || zones("site-a", drift.StateInSync) != 2 ||
		testutil.CollectAndCount(m.ZoneDrifted) != 1 ||
		testutil.ToFloat64(m.ZoneDrifted.WithLabelValues("site-b", "a.example.", drift.StateMissing)) != 1 {
		t.Errorf("after site-a recovered:\n%s", gathered(t, m))
	}
	for _, want := range []string{"a server group couldn't be read", "couldn't read NetBox", `"msg":"drift refreshed"`} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("no %q in the logs:\n%s", want, logs)
		}
	}
}

// gathered returns m's drift metrics, for a failure message.
func gathered(t *testing.T, m *metrics.Metrics) string {
	t.Helper()
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", http.NoBody))
	var out []string
	for l := range strings.Lines(rec.Body.String()) {
		if strings.HasPrefix(l, "nbpdns_drift_zone") || strings.HasPrefix(l, "nbpdns_server_group_up") || strings.HasPrefix(l, "nbpdns_netbox_up") {
			out = append(out, l)
		}
	}
	return strings.Join(out, "")
}

func TestEachRefreshIsATrace(t *testing.T) {
	s, logs, spans := testService(t, Options{Refresh: func(context.Context) (drift.Report, error) { return report(group("a", drift.StateInSync)), nil }})
	// The service's context has a span of its own, as `nbpdns serve`'s
	// command span; each refresh must be a root, not its child.
	ctx, parent := s.o.Tracer.Start(t.Context(), "nbpdns serve")
	defer parent.End()
	s.refresh(ctx, time.Now())
	s.refresh(ctx, time.Now())
	ended := spans.Ended()
	if len(ended) != 2 || ended[0].Name() != "drift refresh" || ended[0].Parent().IsValid() ||
		ended[0].SpanContext().TraceID() == ended[1].SpanContext().TraceID() {
		t.Fatalf("spans %v", ended)
	}
	ids := map[string]bool{}
	for l := range strings.Lines(logs.String()) {
		var rec map[string]any
		if err := json.Unmarshal([]byte(l), &rec); err != nil {
			t.Fatal(err)
		}
		if rec["msg"] == "drift refreshed" {
			ids[rec["request_id"].(string)+"/"+rec["trace_id"].(string)] = true
		}
	}
	if len(ids) != 2 {
		t.Errorf("the refreshes' log lines carry %d request and trace IDs, want 2:\n%s", len(ids), logs)
	}
}

func TestGroups(t *testing.T) {
	groups := []Group{
		{Name: "site-a", URL: "https://a", Views: []string{"_default_"}, DriftPolicy: "report"},
		{Name: "site-b", URL: "https://b", Views: []string{"internal"}, DriftPolicy: "enforce"},
	}
	s, _, _ := testService(t, Options{Groups: groups})
	// Before any refresh: the configuration, and nothing known.
	for i, g := range s.Groups() {
		if !reflect.DeepEqual(g.Group, groups[i]) || g.Info.Status != statusUnknown || g.Report != nil || g.Info.LastSuccess != nil {
			t.Errorf("before a refresh, %+v", g)
		}
	}
	t0 := time.Now()
	s.record(t.Context(), report(group("site-a", drift.StateDrift), group("site-b", drift.StateInSync)), nil, t0, t0)
	// site-b's primary fails: it keeps its report, and says it failed.
	s.record(t.Context(), report(group("site-a", drift.StateInSync), failedGroup("site-b")), nil, t0, t0)
	got := s.Groups()
	a, b := got[0], got[1]
	if a.Report == nil || a.Report.Counts.InSync != 1 || a.Info.Status != drift.StatusOK {
		t.Errorf("site-a: %+v", a)
	}
	if b.Report == nil || b.Report.Counts.InSync != 1 || b.Info.Status != drift.StatusFailed || b.Info.Error == "" || b.Info.LastSuccess == nil {
		t.Errorf("site-b: %+v", b)
	}
}

func TestNetBoxZones(t *testing.T) {
	s, _, _ := testService(t, Options{})
	if nb := s.NetBox(); !nb.AsOf.IsZero() || nb.Zones != nil {
		t.Errorf("before a refresh: %+v", nb)
	}
	t0 := time.Now()
	r := report(group("site-a", drift.StateInSync))
	r.NetBox = []dns.Zone{{Name: "a.example.", View: "v", Active: true}, {Name: "a.example.", View: "w", Active: true}, {Name: "b.example.", View: "w", Active: true}}
	s.record(t.Context(), r, nil, t0, t0)
	nb := s.NetBox()
	if !nb.AsOf.Equal(t0) || len(nb.Zones["v"]) != 1 || len(nb.Zones["w"]) != 2 {
		t.Fatalf("after a refresh: %+v", nb)
	}
	// The first of the views, by name, that has the zone.
	if z, ok := nb.Zone([]string{"w", "v"}, "a.example."); !ok || z.View != "v" {
		t.Errorf("Zone(w, v) = %+v, %v", z, ok)
	}
	if _, ok := nb.Zone([]string{"v"}, "b.example."); ok {
		t.Error("b.example. is only in view w")
	}
	// NetBox fails, and a refresh times out: the zones stay as they were.
	t1 := t0.Add(time.Minute)
	s.record(t.Context(), drift.Report{}, errors.New("NetBox isn't reachable"), t1, t1)
	s.record(t.Context(), drift.Report{}, &TimeoutError{Timeout: time.Minute}, t1, t1)
	if nb := s.NetBox(); !nb.AsOf.Equal(t0) || len(nb.Zones["w"]) != 2 {
		t.Errorf("after failures: %+v", nb)
	}
}
