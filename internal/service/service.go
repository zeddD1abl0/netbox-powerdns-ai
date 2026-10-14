// Package service runs nbpdns continuously (ADR-0029). It refreshes the
// drift report on a schedule, keeps each server group's last-known state,
// sets the drift metrics, and serves the health checks and the metrics.
package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/logging"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/metrics"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/version"
)

// historyWriteTimeout bounds writing a refresh to the drift history, which
// happens after the refresh, whatever was left of its drift.timeout.
const historyWriteTimeout = 30 * time.Second

// A History keeps each refresh, and gives back the last reports at start
// (ADR-0042). The store is one.
type History interface {
	RecordRefresh(ctx context.Context, r store.RefreshRecord) (int, []audit.Event, error)
	RestoreDrift(ctx context.Context) (store.Restored, error)
}

// A RefreshFunc reads NetBox and every group's primary, and compares them,
// as `nbpdns drift` does: every zone, or, if zones isn't nil, only those, as
// drift.Options.Zones does (ADR-0036). Its error means NetBox couldn't be
// read.
type RefreshFunc func(ctx context.Context, zones []drift.ZoneRef) (drift.Report, error)

// Options configure a Service.
type Options struct {
	Refresh RefreshFunc
	// Interval is the time from one refresh's start to the next's.
	Interval time.Duration
	// Timeout bounds a refresh.
	Timeout time.Duration
	// WebhookDelay is how long the zones that NetBox's webhooks name wait
	// for the webhooks to stop: drift.webhook_delay. Zero is 3 seconds.
	WebhookDelay time.Duration
	// Webhooks says whether the API takes NetBox's webhooks, for /status.
	Webhooks bool
	// History, if set, keeps each refresh, and restores the last reports
	// at start; DatabasePath is where, for /status.
	History      History
	DatabasePath string
	Log          *slog.Logger
	Tracer       trace.Tracer
	Metrics      *metrics.Metrics

	// What /status and the API show besides the refreshes: the build,
	// NetBox's URL, each group, in the configuration's order, and where
	// spans are exported, if they are.
	Version   version.Info
	NetBoxURL string
	Groups    []Group
	OTLP      OTLP

	now func() time.Time // time.Now if nil
	// maxWait bounds how long webhooks' zones wait, from the first. Zero is
	// 30 seconds.
	maxWait time.Duration
}

// A Group is a server group's configuration, as /status and the API show
// it.
type Group struct {
	Name string
	// URL is the group's primary's.
	URL   string
	Views []string
	// DriftPolicy is the policy of the group's zones that it doesn't name
	// a policy for.
	DriftPolicy string
}

// OTLP says where spans are exported. An empty Endpoint means nowhere.
type OTLP struct {
	Endpoint string
	Protocol string
}

// Timing is the refreshes' schedule, which can change while the service
// runs (ADR-0040).
type Timing struct {
	// Interval is the time from one refresh's start to the next's.
	Interval time.Duration
	// Timeout bounds a refresh.
	Timeout time.Duration
	// WebhookDelay is how long the zones that NetBox's webhooks name wait
	// for the webhooks to stop. Zero is 3 seconds.
	WebhookDelay time.Duration
}

// A Service refreshes the drift report, and keeps what it found.
type Service struct {
	o       Options
	started time.Time
	timing  atomic.Pointer[Timing]
	// webhooksOn is whether the API takes NetBox's webhooks, for /status.
	webhooksOn atomic.Bool
	// views are the views that the groups serve.
	views map[string]bool
	// wake tells Run that a webhook has queued a refresh.
	wake chan struct{}

	mu sync.Mutex
	st state
}

// state is what the refreshes found, guarded by Service.mu.
type state struct {
	// ready is set once the first refresh has finished, or the last
	// reports are restored.
	ready bool
	// restored is set if the last reports were restored at start, and
	// restoredAsOf is when the refresh before the restart finished.
	restored     bool
	restoredAsOf time.Time
	// writeFailures counts the refreshes that couldn't be written to the
	// history, and lastWriteError is the last one's error, at
	// lastWriteFailure.
	writeFailures    int
	lastWriteError   string
	lastWriteFailure time.Time
	lastStart        time.Time
	lastEnd          time.Time
	// lastOutcome is the last refresh's: a metrics.Outcome constant.
	lastOutcome string
	// lastError is why the last refresh failed, if it did.
	lastError    string
	lastComplete time.Time
	next         time.Time
	refreshes    map[string]int
	netboxUp     bool
	netboxError  string
	groups       map[string]*groupState
	// netbox is NetBox's zones as of its last successful read.
	netbox NetBoxView
	// pending is what NetBox's webhooks asked to refresh, waiting.
	pending batch
	// lastEvent is the last webhook's event, and lastWebhookRefresh the
	// last refresh that webhooks asked for, or nil.
	lastEvent          *Event
	lastWebhookRefresh *WebhookRefresh
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
	if o.maxWait == 0 {
		o.maxWait = maxWait
	}
	views := map[string]bool{}
	for _, g := range o.Groups {
		for _, v := range g.Views {
			views[v] = true
		}
	}
	s := &Service{o: o, started: o.now(), views: views, wake: make(chan struct{}, 1),
		st: state{refreshes: map[string]int{}, groups: map[string]*groupState{}}}
	s.storeTiming(Timing{Interval: o.Interval, Timeout: o.Timeout, WebhookDelay: o.WebhookDelay})
	s.webhooksOn.Store(o.Webhooks)
	return s
}

// SetWebhooks says whether the API takes NetBox's webhooks, as
// netbox.webhook_secret, which can change while the service runs, says.
func (s *Service) SetWebhooks(on bool) { s.webhooksOn.Store(on) }

// Timing returns the refreshes' schedule.
func (s *Service) Timing() Timing { return *s.timing.Load() }

// SetTiming changes the refreshes' schedule: the next refresh starts an
// interval after the last one started, and a refresh that starts from now
// is bounded by the timeout, and webhooks' zones wait for the delay.
func (s *Service) SetTiming(t Timing) {
	s.storeTiming(t)
	s.mu.Lock()
	if st := &s.st; !st.next.IsZero() && !st.lastStart.After(st.lastEnd) {
		st.next = maxTime(st.lastStart.Add(t.Interval), s.o.now())
	}
	s.mu.Unlock()
	// Run waits until the next refresh; it works the time out again.
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) storeTiming(t Timing) {
	if t.WebhookDelay == 0 {
		t.WebhookDelay = 3 * time.Second
	}
	s.timing.Store(&t)
}

// Run refreshes at once, then every interval, from one refresh's start to
// the next's, until ctx is canceled. In between, it refreshes the zones that
// NetBox's webhooks name, once they're due (ADR-0036). Refreshes never
// overlap: one that takes longer than the interval delays the next.
func (s *Service) Run(ctx context.Context) {
	// last is the last full refresh's start, and the next one starts an
	// interval after it, whatever the interval is by then.
	var last time.Time
	next := func() time.Time {
		if last.IsZero() {
			return time.Time{}
		}
		return last.Add(s.Timing().Interval)
	}
	for {
		b, full := s.wait(ctx, next)
		if ctx.Err() != nil {
			return
		}
		start := s.o.now()
		if !full {
			s.refreshZones(ctx, start, b)
			continue
		}
		s.refresh(ctx, start, b)
		if ctx.Err() != nil {
			return
		}
		last = start
		n := next()
		if now := s.o.now(); now.After(n) {
			s.o.Log.WarnContext(ctx, "the drift refresh took longer than drift.interval, so the next one starts now",
				"duration_seconds", now.Sub(start).Seconds(), "interval_seconds", s.Timing().Interval.Seconds())
			n = now
		}
		s.mu.Lock()
		s.st.next = n
		s.mu.Unlock()
	}
}

// wait waits until next(), when the scheduled refresh is due, which is now
// if it's zero, or until what webhooks queued is due, whichever is sooner,
// and takes what's queued. full reports whether a full refresh is due: the
// scheduled one, which covers every zone queued, or one that a webhook
// asked for. A wake, from a webhook or a change of timing, works the time
// out again.
func (s *Service) wait(ctx context.Context, nextFn func() time.Time) (b batch, full bool) {
	for {
		now := s.o.now()
		next := nextFn()
		until := next
		s.mu.Lock()
		if p := &s.st.pending; p.events > 0 {
			until = minTime(until, p.due(s.Timing().WebhookDelay, s.o.maxWait))
		}
		s.mu.Unlock()
		if !until.After(now) {
			b := s.take()
			return b, !now.Before(next) || b.full
		}
		t := time.NewTimer(until.Sub(now))
		select {
		case <-ctx.Done():
			t.Stop()
			return batch{}, false
		case <-s.wake:
			t.Stop()
		case <-t.C:
		}
	}
}

// refresh runs one full refresh, as its own trace with its own request ID,
// and records it. If webhooks queued b, the refresh covers it: its trace
// links to their spans, and its logs carry their NetBox requests. A
// refresh cut short by ctx's cancellation isn't recorded.
func (s *Service) refresh(ctx context.Context, start time.Time, b batch) {
	s.mu.Lock()
	s.st.lastStart = start
	s.mu.Unlock()
	log := s.o.Log
	opts := []trace.SpanStartOption{trace.WithNewRoot()}
	if b.events > 0 {
		log = log.With("netbox_request_ids", b.requestIDs())
		opts = append(opts, trace.WithLinks(b.links...), trace.WithAttributes(b.attributes()...))
	}
	rctx := logging.WithRequestID(ctx, rand.Text())
	rctx, span := s.o.Tracer.Start(rctx, "drift refresh", opts...)
	defer span.End()
	if b.full {
		log.InfoContext(rctx, "refreshing every zone, as NetBox's webhooks asked", "reason", b.reason, "events", b.events)
	}
	timeout := s.Timing().Timeout
	rctx, cancel := context.WithTimeout(rctx, timeout)
	defer cancel()
	r, err := s.o.Refresh(rctx, nil)
	if ctx.Err() != nil {
		return
	}
	// A refresh stopped by its timeout says nothing about NetBox or the
	// primaries, though the reads it stopped failed.
	if errors.Is(rctx.Err(), context.DeadlineExceeded) && (err != nil || !r.Complete) {
		err = &TimeoutError{Timeout: timeout}
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	end := s.o.now()
	outcome, changes := s.record(rctx, log, r, err, start, end)
	if b.events > 0 {
		s.noteWebhookRefresh(b, true, start, end, outcome, err)
	}
	cause := store.CauseSchedule
	if b.full {
		cause = store.CauseWebhook
	}
	s.keep(rctx, log, store.RefreshRecord{Kind: store.RefreshFull, Cause: cause, Started: start, Finished: end,
		Outcome: outcome, Error: errText(err), NetBoxRequestIDs: b.requestIDs()}, changes)
}

// keep writes a refresh, rec, and what it kept, changes, to the history, if
// there's one. A failed write is counted, logged and shown on /status; the
// refresh is still served, and the next one writes the zones' changes since
// their last rows.
func (s *Service) keep(ctx context.Context, log *slog.Logger, rec store.RefreshRecord, changes []change) {
	if s.o.History == nil {
		return
	}
	rec.TraceID = trace.SpanContextFromContext(ctx).TraceID().String()
	for _, c := range changes {
		g := c.g
		if c.failed {
			rec.Groups = append(rec.Groups, store.GroupResult{Group: g.Group, Status: drift.StatusFailed, Error: g.Error})
			continue
		}
		rec.Groups = append(rec.Groups, store.GroupResult{Group: g.Group, Status: drift.StatusOK, Counts: g.Counts})
		rec.Reports = append(rec.Reports, store.KeptReport{Report: g, LastSuccess: c.lastSuccess})
	}
	// The refresh's own timeout may have passed.
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), historyWriteTimeout)
	defer cancel()
	rows, events, err := s.o.History.RecordRefresh(wctx, rec)
	if err != nil {
		s.o.Metrics.DBWriteFailure(metrics.WriteHistory)
		s.mu.Lock()
		s.st.writeFailures++
		s.st.lastWriteError, s.st.lastWriteFailure = err.Error(), s.o.now()
		s.mu.Unlock()
		log.ErrorContext(wctx, "couldn't write the refresh to the drift history; it's still served, and the next refresh writes the zones' changes",
			"err", err)
		return
	}
	s.o.Metrics.AuditEvents(events)
	log.DebugContext(wctx, "wrote the refresh to the drift history", "zone_rows", rows, "audit_events", len(events))
}

// errText returns err's text, or empty if it's nil.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// TimeoutError is the error of a refresh stopped by drift.timeout.
type TimeoutError struct{ Timeout time.Duration }

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("the refresh took longer than drift.timeout, %s, so it was stopped", e.Timeout)
}

// record keeps what a full refresh found, sets the metrics, logs it to
// log, and returns its outcome, and what it kept for each group. A group
// that couldn't be read keeps its last report, and if NetBox couldn't be
// read, or the refresh ran out of time, every group does.
func (s *Service) record(ctx context.Context, log *slog.Logger, r drift.Report, err error, start, end time.Time) (string, []change) {
	m := s.o.Metrics
	outcome := outcomeOf(r, err)
	m.Refreshes.WithLabelValues(outcome).Inc()
	m.RefreshDuration.Observe(end.Sub(start).Seconds())
	m.LastRefresh.WithLabelValues().Set(unix(end))
	if outcome == metrics.OutcomeComplete {
		m.LastCompleteRefresh.WithLabelValues().Set(unix(end))
	}

	s.mu.Lock()
	st := &s.st
	st.ready, st.lastEnd, st.lastOutcome, st.lastError = true, end, outcome, ""
	st.refreshes[outcome]++
	if outcome == metrics.OutcomeComplete {
		st.lastComplete = end
	}
	var te *TimeoutError
	if errors.As(err, &te) {
		st.lastError = err.Error()
		s.mu.Unlock()
		log.WarnContext(ctx, "the drift refresh took longer than drift.timeout, so it was stopped, and every server group keeps its last report",
			"timeout_seconds", te.Timeout.Seconds())
		return outcome, nil
	}
	if err != nil {
		st.lastError = err.Error()
		st.netboxUp, st.netboxError = false, err.Error()
		s.mu.Unlock()
		m.NetBoxUp.WithLabelValues().Set(0)
		log.WarnContext(ctx, "the drift refresh couldn't read NetBox, so every server group keeps its last report", "err", err)
		return outcome, nil
	}
	st.netboxUp, st.netboxError = true, ""
	if r.NetBox != nil {
		st.netbox = netboxView(r.NetBox, end)
	}
	if r.NetBoxErr != nil {
		log.WarnContext(ctx, "couldn't read the records of NetBox's zones that aren't compared, so the API keeps NetBox's last records",
			"err", r.NetBoxErr)
	}
	changes := s.apply(r.Groups, end, false)
	s.mu.Unlock()

	m.NetBoxUp.WithLabelValues().Set(1)
	s.publish(ctx, log, changes, end)
	return outcome, changes
}

// A change is what a refresh found for a group, kept, for publish to set
// the metrics and log once the lock is released.
type change struct {
	// g is the report kept: for a zone refresh, the group's last report
	// with the zone refresh's merged in.
	g       drift.GroupReport
	failed  bool
	now     map[[2]string]bool
	removed [][2]string
	// zones are those that a zone refresh compared, or nil.
	zones []string
	// lastSuccess is the group's last full comparison's time.
	lastSuccess time.Time
}

// apply keeps each group's report from groups, read at end, and returns the
// changes. A group that failed keeps its last report. With merge, groups
// are a zone refresh's, which are merged into the last reports; a group
// with no report waits for the next full refresh. It's called with s.mu
// held.
func (s *Service) apply(groups []drift.GroupReport, end time.Time, merge bool) []change {
	var changes []change
	for _, g := range groups {
		gs := s.st.groups[g.Group]
		if gs == nil {
			if merge {
				continue
			}
			gs = &groupState{drifted: map[[2]string]bool{}}
			s.st.groups[g.Group] = gs
		}
		if g.Status != drift.StatusOK {
			gs.up, gs.lastError = false, g.Error
			changes = append(changes, change{g: g, failed: true})
			continue
		}
		zones := g.Compared
		if merge {
			if gs.lastSuccess.IsZero() {
				continue
			}
			g = drift.Merge(gs.report, g)
		}
		// The last success is the last full comparison's: a zone refresh
		// compares only some zones, and the rest of the report is as old.
		gs.up, gs.lastError, gs.report = true, "", g
		if !merge {
			gs.lastSuccess = end
		}
		now := drifted(g)
		var removed [][2]string
		for k := range gs.drifted {
			if !now[k] {
				removed = append(removed, k)
			}
		}
		gs.drifted = now
		changes = append(changes, change{g: g, now: now, removed: removed, zones: zones, lastSuccess: gs.lastSuccess})
	}
	return changes
}

// publish sets each changed group's metrics, and logs it to log. A zone
// refresh logs only its own zones' warnings and drift.
func (s *Service) publish(ctx context.Context, log *slog.Logger, changes []change, end time.Time) {
	m := s.o.Metrics
	for _, c := range changes {
		g := c.g
		if c.failed {
			m.GroupUp.WithLabelValues(g.Group).Set(0)
			log.WarnContext(ctx, "a server group couldn't be read, so it keeps its last report", "group", g.Group, "err", g.Error)
			continue
		}
		m.GroupUp.WithLabelValues(g.Group).Set(1)
		if c.zones == nil {
			m.GroupLastSuccess.WithLabelValues(g.Group).Set(unix(end))
		}
		setGroup(m, g, c.now, c.removed)
		cs := g.Counts
		args := []any{"group", g.Group, "in_sync", cs.InSync, "drift", cs.Drift,
			"missing", cs.Missing, "inactive_in_netbox", cs.Inactive, "ignored", cs.Ignored, "unmanaged", cs.Unmanaged}
		ours := func(string) bool { return true }
		if c.zones != nil {
			ours = func(zone string) bool { return slices.Contains(c.zones, zone) }
			log.InfoContext(ctx, "zones refreshed", append(args, "zones", c.zones)...)
		} else {
			log.InfoContext(ctx, "drift refreshed", args...)
		}
		for _, w := range g.Warnings {
			if ours(w.Zone) {
				log.WarnContext(ctx, "the drift report worked around a problem", "group", g.Group, "warning", w.Text)
			}
		}
		for _, z := range g.Zones {
			if drift.IsDrifted(z.State) && ours(z.Zone) {
				log.DebugContext(ctx, "zone drifted", "group", g.Group, "zone", z.Zone, "state", z.State, "changes", len(z.Changes))
			}
		}
	}
}

// setGroup sets a compared group's drift metrics. now are its drifted
// zones, and removed the drifted-zone series of zones that no longer
// drifted. A series that stays is set again, never removed first, so that
// no scrape misses it.
func setGroup(m *metrics.Metrics, g drift.GroupReport, now map[[2]string]bool, removed [][2]string) {
	for state, n := range g.Counts.ByState() {
		m.Zones.WithLabelValues(g.Group, state).Set(float64(n))
	}
	kinds := map[string]int{}
	for _, z := range g.Zones {
		for _, ch := range z.Changes {
			kinds[ch.Kind]++
		}
	}
	for _, kind := range drift.ChangeKinds {
		m.RRsetChanges.WithLabelValues(g.Group, kind).Set(float64(kinds[kind]))
	}
	m.Problems.WithLabelValues(g.Group).Set(float64(len(g.Problems)))
	m.Warnings.WithLabelValues(g.Group).Set(float64(len(g.Warnings)))
	for _, k := range removed {
		m.ZoneDrifted.DeleteLabelValues(g.Group, k[0], k[1])
	}
	for k := range now {
		m.ZoneDrifted.WithLabelValues(g.Group, k[0], k[1]).Set(1)
	}
}

// drifted returns the zone and state of each of g's drifted zones.
func drifted(g drift.GroupReport) map[[2]string]bool {
	out := map[[2]string]bool{}
	for _, z := range g.Zones {
		if drift.IsDrifted(z.State) {
			out[[2]string{z.Zone, z.State}] = true
		}
	}
	return out
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
// service started if none has since, within the time a refresh can take. A
// restored refresh from before the start doesn't count.
func (s *Service) alive(last time.Time) (bool, time.Time) {
	if last.Before(s.started) {
		last = s.started
	}
	t := s.Timing()
	return s.o.now().Sub(last) <= t.Interval+t.Timeout+time.Minute, last
}

// readyz answers 200 once the first refresh has finished, whatever its
// outcome (ADR-0029), or once the last reports are restored (ADR-0042).
func (s *Service) readyz(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	ready := s.st.ready
	s.mu.Unlock()
	if !ready {
		text(w, http.StatusServiceUnavailable, "not ready: the first drift refresh hasn't finished, and there was no report to restore")
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
