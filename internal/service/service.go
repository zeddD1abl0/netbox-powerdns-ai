// Package service runs nbpdns continuously (ADR-0029). It refreshes the
// drift report on a schedule, keeps each server group's last-known state,
// sets the drift metrics, and serves the health checks and the metrics.
package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/logging"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/metrics"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/version"
)

// A RefreshFunc reads NetBox and every group's primary, and compares them,
// as `nbpdns drift` does. Its error means NetBox couldn't be read.
type RefreshFunc func(ctx context.Context) (drift.Report, error)

// Options configure a Service.
type Options struct {
	Refresh RefreshFunc
	// Interval is the time from one refresh's start to the next's.
	Interval time.Duration
	// Timeout bounds a refresh.
	Timeout time.Duration
	Log     *slog.Logger
	Tracer  trace.Tracer
	Metrics *metrics.Metrics

	// What /status shows besides the refreshes: the build, NetBox's URL,
	// each group's primary, in the configuration's order, and where spans
	// are exported, if they are.
	Version   version.Info
	NetBoxURL string
	Groups    []Primary
	OTLP      OTLP

	now func() time.Time // time.Now if nil
}

// A Primary is a server group and its primary's URL.
type Primary struct {
	Group string
	URL   string
}

// OTLP says where spans are exported. An empty Endpoint means nowhere.
type OTLP struct {
	Endpoint string
	Protocol string
}

// A Service refreshes the drift report, and keeps what it found.
type Service struct {
	o       Options
	started time.Time

	mu sync.Mutex
	st state
}

// state is what the refreshes found, guarded by Service.mu.
type state struct {
	// ready is set once the first refresh has finished.
	ready     bool
	lastStart time.Time
	lastEnd   time.Time
	// lastOutcome is the last refresh's: a metrics.Outcome constant.
	lastOutcome  string
	lastComplete time.Time
	next         time.Time
	refreshes    map[string]int
	netboxUp     bool
	netboxError  string
	groups       map[string]*groupState
}

// groupState is a server group's last-known state.
type groupState struct {
	up          bool
	lastSuccess time.Time
	lastError   string
	// report is the group's last successful comparison.
	report drift.GroupReport
	// drifted are the zone and state of each drifted-zone series it set.
	drifted map[[2]string]bool
}

// New returns a Service. Run it with Run, and serve its Handler.
func New(o Options) *Service {
	if o.now == nil {
		o.now = time.Now
	}
	return &Service{o: o, started: o.now(), st: state{refreshes: map[string]int{}, groups: map[string]*groupState{}}}
}

// Run refreshes at once, then every interval, from one refresh's start to
// the next's, until ctx is canceled. A refresh that takes longer than the
// interval delays the next one, so refreshes never overlap.
func (s *Service) Run(ctx context.Context) {
	for {
		start := s.o.now()
		s.refresh(ctx, start)
		if ctx.Err() != nil {
			return
		}
		next := start.Add(s.o.Interval)
		if now := s.o.now(); now.After(next) {
			s.o.Log.WarnContext(ctx, "the drift refresh took longer than drift.interval, so the next one starts now",
				"duration_seconds", now.Sub(start).Seconds(), "interval_seconds", s.o.Interval.Seconds())
			next = now
		}
		s.mu.Lock()
		s.st.next = next
		s.mu.Unlock()
		t := time.NewTimer(next.Sub(s.o.now()))
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
}

// refresh runs one refresh, as its own trace with its own request ID, and
// records it. A refresh cut short by ctx's cancellation isn't recorded.
func (s *Service) refresh(ctx context.Context, start time.Time) {
	s.mu.Lock()
	s.st.lastStart = start
	s.mu.Unlock()
	rctx := logging.WithRequestID(ctx, rand.Text())
	rctx, span := s.o.Tracer.Start(rctx, "drift refresh", trace.WithNewRoot())
	defer span.End()
	rctx, cancel := context.WithTimeout(rctx, s.o.Timeout)
	defer cancel()
	r, err := s.o.Refresh(rctx)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	s.record(rctx, r, err, start, s.o.now())
}

// record keeps what a refresh found, sets the metrics, and logs it. A group
// that couldn't be read keeps its last report, and if NetBox couldn't be
// read, every group does.
func (s *Service) record(ctx context.Context, r drift.Report, err error, start, end time.Time) {
	m := s.o.Metrics
	outcome := metrics.OutcomeComplete
	switch {
	case err != nil:
		outcome = metrics.OutcomeFailed
	case !r.Complete:
		outcome = metrics.OutcomeIncomplete
	}
	m.Refreshes.WithLabelValues(outcome).Inc()
	m.RefreshDuration.Observe(end.Sub(start).Seconds())
	m.LastRefresh.Set(unix(end))
	if outcome == metrics.OutcomeComplete {
		m.LastCompleteRefresh.Set(unix(end))
	}

	s.mu.Lock()
	st := &s.st
	st.ready, st.lastEnd, st.lastOutcome = true, end, outcome
	st.refreshes[outcome]++
	if outcome == metrics.OutcomeComplete {
		st.lastComplete = end
	}
	if err != nil {
		st.netboxUp, st.netboxError = false, err.Error()
		s.mu.Unlock()
		m.NetBoxUp.Set(0)
		s.o.Log.WarnContext(ctx, "the drift refresh couldn't read NetBox, so every server group keeps its last report", "err", err)
		return
	}
	st.netboxUp, st.netboxError = true, ""
	type change struct {
		g       drift.GroupReport
		removed [][2]string
	}
	var changes []change
	for _, g := range r.Groups {
		gs := st.groups[g.Group]
		if gs == nil {
			gs = &groupState{drifted: map[[2]string]bool{}}
			st.groups[g.Group] = gs
		}
		if g.Status != drift.StatusOK {
			gs.up, gs.lastError = false, g.Error
			changes = append(changes, change{g: g})
			continue
		}
		gs.up, gs.lastError, gs.lastSuccess, gs.report = true, "", end, g
		now := drifted(g)
		var removed [][2]string
		for k := range gs.drifted {
			if !now[k] {
				removed = append(removed, k)
			}
		}
		gs.drifted = now
		changes = append(changes, change{g: g, removed: removed})
	}
	s.mu.Unlock()

	m.NetBoxUp.Set(1)
	for _, c := range changes {
		g := c.g
		if g.Status != drift.StatusOK {
			m.GroupUp.WithLabelValues(g.Group).Set(0)
			s.o.Log.WarnContext(ctx, "a server group couldn't be read, so it keeps its last report", "group", g.Group, "err", g.Error)
			continue
		}
		m.GroupUp.WithLabelValues(g.Group).Set(1)
		m.GroupLastSuccess.WithLabelValues(g.Group).Set(unix(end))
		setGroup(m, g, c.removed)
		cs := g.Counts
		s.o.Log.InfoContext(ctx, "drift refreshed", "group", g.Group, "in_sync", cs.InSync, "drift", cs.Drift,
			"missing", cs.Missing, "inactive_in_netbox", cs.Inactive, "ignored", cs.Ignored, "unmanaged", cs.Unmanaged)
		for _, w := range g.Warnings {
			s.o.Log.WarnContext(ctx, "the drift report worked around a problem", "group", g.Group, "warning", w)
		}
		for _, z := range g.Zones {
			if isDrifted(z.State) {
				s.o.Log.DebugContext(ctx, "zone drifted", "group", g.Group, "zone", z.Zone, "state", z.State, "changes", len(z.Changes))
			}
		}
	}
}

// setGroup sets a compared group's drift metrics, and removes the
// drifted-zone series of zones that no longer drifted. A series that
// stays is set again, never removed first, so that no scrape misses it.
func setGroup(m *metrics.Metrics, g drift.GroupReport, removed [][2]string) {
	c := g.Counts
	for state, n := range map[string]int{
		drift.StateInSync: c.InSync, drift.StateDrift: c.Drift, drift.StateMissing: c.Missing,
		drift.StateInactive: c.Inactive, drift.StateIgnored: c.Ignored, "unmanaged": c.Unmanaged,
	} {
		m.Zones.WithLabelValues(g.Group, state).Set(float64(n))
	}
	kinds := map[string]int{drift.ChangeMissing: 0, drift.ChangeExtra: 0, drift.ChangeChanged: 0}
	for _, z := range g.Zones {
		for _, ch := range z.Changes {
			kinds[ch.Kind]++
		}
	}
	for kind, n := range kinds {
		m.RRsetChanges.WithLabelValues(g.Group, kind).Set(float64(n))
	}
	m.Problems.WithLabelValues(g.Group).Set(float64(len(g.Problems)))
	m.Warnings.WithLabelValues(g.Group).Set(float64(len(g.Warnings)))
	for _, k := range removed {
		m.ZoneDrifted.DeleteLabelValues(g.Group, k[0], k[1])
	}
	for k := range drifted(g) {
		m.ZoneDrifted.WithLabelValues(g.Group, k[0], k[1]).Set(1)
	}
}

// drifted returns the zone and state of each of g's drifted zones.
func drifted(g drift.GroupReport) map[[2]string]bool {
	out := map[[2]string]bool{}
	for _, z := range g.Zones {
		if isDrifted(z.State) {
			out[[2]string{z.Zone, z.State}] = true
		}
	}
	return out
}

func isDrifted(state string) bool {
	return state == drift.StateDrift || state == drift.StateMissing || state == drift.StateInactive
}

func unix(t time.Time) float64 { return float64(t.UnixNano()) / 1e9 }

// Handler serves /livez, /readyz, /status and /metrics. GET and HEAD work;
// another method gets 405, and another path 404.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", s.livez)
	mux.HandleFunc("GET /readyz", s.readyz)
	mux.HandleFunc("GET /status", s.status)
	mux.Handle("GET /metrics", s.o.Metrics.Handler())
	return mux
}

// livez answers 200 unless no refresh has started for longer than a
// refresh can take, which means the loop is stuck.
func (s *Service) livez(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	last := s.st.lastStart
	s.mu.Unlock()
	if alive, since := s.alive(last); !alive {
		text(w, http.StatusServiceUnavailable, fmt.Sprintf("stuck: no drift refresh has started since %s", since.UTC().Format(time.RFC3339)))
		return
	}
	text(w, http.StatusOK, "ok")
}

// alive reports whether a refresh has started, since last, or since the
// service started if none has, within the time a refresh can take.
func (s *Service) alive(last time.Time) (bool, time.Time) {
	if last.IsZero() {
		last = s.started
	}
	return s.o.now().Sub(last) <= s.o.Interval+s.o.Timeout+time.Minute, last
}

// readyz answers 200 once the first refresh has finished, whatever its
// outcome (ADR-0029).
func (s *Service) readyz(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	ready := s.st.ready
	s.mu.Unlock()
	if !ready {
		text(w, http.StatusServiceUnavailable, "not ready: the first drift refresh hasn't finished")
		return
	}
	text(w, http.StatusOK, "ready")
}

func text(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = fmt.Fprintln(w, body)
}
