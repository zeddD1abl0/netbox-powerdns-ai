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

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/logging"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/metrics"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/version"
)

// historyWriteTimeout bounds writing a refresh to the drift history, which
// happens after the refresh, whatever was left of its drift.timeout.
const historyWriteTimeout = 30 * time.Second

// electionStall is how long a follower's election may go without polling
// before /livez calls it stuck.
const electionStall = time.Minute

// The roles of a replica (ADR-0047).
const (
	RoleLeader   = "leader"
	RoleFollower = "follower"
)

// A History keeps each refresh, and gives back what the refreshes found, at
// start and whenever another replica's refresh is recorded (ADR-0042,
// ADR-0048). The store is one.
type History interface {
	RecordRefresh(ctx context.Context, r store.RefreshRecord) (store.Recorded, error)
	LoadDrift(ctx context.Context, since int64) (store.DriftState, error)
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
	// History, if set, keeps each refresh, which the service serves once
	// it's kept, and gives back what the refreshes found, which the service
	// serves; Database says where, for /status.
	History  History
	Database DatabaseInfo
	// Replica is this serve, among those on the database. Elected says
	// whether an election chooses the leader among them, on PostgreSQL,
	// which reports to Lead and Follow, and runs Run while this replica
	// leads (ADR-0047). Otherwise, on SQLite, the service always leads.
	Replica store.Replica
	Elected bool
	Log     *slog.Logger
	Tracer  trace.Tracer
	Metrics *metrics.Metrics

	// What /status and the API show besides the refreshes: the build,
	// NetBox's URL, each group, in the configuration's order, which
	// SetGroups changes, and where spans are exported, if they are.
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
	// ID is the group's in the database, which a rename keeps (ADR-0045),
	// or zero if it has none.
	ID   int64
	Name string
	// URL is the group's primary's.
	URL   string
	Views []string
	// DriftPolicy is the policy of the group's zones that it doesn't name
	// a policy for.
	DriftPolicy string
	// ManagedBy is what manages the group: file or cli (ADR-0040).
	ManagedBy string
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
	// wake tells Run that a webhook has queued a refresh, or that the
	// schedule changed.
	wake chan struct{}

	mu sync.Mutex
	// groups are the groups compared, in the configuration's order, and
	// views the views that they serve, which SetGroups changes.
	groups []Group
	views  map[string]bool
	st     state
}

// state is what the refreshes found, guarded by Service.mu.
type state struct {
	// version is the refreshes' version in the history that the state is
	// as of: the last refresh that this replica kept, or loaded (ADR-0048).
	// It's 0 without a history.
	version int64
	// ready is set once the service serves a full refresh that finished:
	// its own, or one loaded from the history.
	ready bool
	// restored is set if serve started from a refresh that the history
	// kept, and restoredAsOf is when that refresh finished.
	restored     bool
	restoredAsOf time.Time
	// writeFailures counts the refreshes that couldn't be written to the
	// history, and lastWriteError is the last one's error, at
	// lastWriteFailure.
	writeFailures    int
	lastWriteError   string
	lastWriteFailure time.Time
	// lastStart is when the last refresh started, which may still run;
	// lastEnd is when the last one finished, which started at lastEndStart.
	lastStart    time.Time
	lastEnd      time.Time
	lastEndStart time.Time
	// lastOutcome is the last refresh's: a metrics.Outcome constant.
	lastOutcome string
	// lastError is why the last refresh failed, if it did.
	lastError    string
	lastComplete time.Time
	next         time.Time
	refreshes    map[string]int
	// netboxRead reports whether a refresh has read NetBox, or tried to, or
	// the restore found one that had, so that netboxUp says something. A
	// refresh that compares no group doesn't read it.
	netboxRead  bool
	netboxUp    bool
	netboxError string
	groups      map[string]*groupState
	// netbox is NetBox's zones as of its last successful read.
	netbox NetBoxView
	// pending is what NetBox's webhooks asked to refresh, waiting.
	pending batch
	// lastEvent is the last webhook's event, and lastWebhookRefresh the
	// last refresh that webhooks asked for, or nil.
	lastEvent          *Event
	lastWebhookRefresh *WebhookRefresh
	// refreshNow brings the next full refresh forward to now.
	refreshNow bool
	// leads is whether this replica leads, which it has since leadSince;
	// leader is the leader as it last knew it, or nil if it knows none; and
	// polled is when the election last reported.
	leads     bool
	leadSince time.Time
	leader    *store.Leader
	polled    time.Time
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
	s := &Service{o: o, started: o.now(), groups: o.Groups, views: viewsOf(o.Groups), wake: make(chan struct{}, 1),
		st: state{refreshes: map[string]int{}, groups: map[string]*groupState{}}}
	s.storeTiming(Timing{Interval: o.Interval, Timeout: o.Timeout, WebhookDelay: o.WebhookDelay})
	s.webhooksOn.Store(o.Webhooks)
	if !o.Elected {
		s.st.leads, s.st.leadSince = true, s.started
	}
	return s
}

// Lead records that this replica leads, with l, its row (ADR-0047). The
// election calls it when the replica takes over, and at each heartbeat.
func (s *Service) Lead(l store.Leader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.o.now()
	if !s.st.leads {
		s.st.leads, s.st.leadSince = true, now
	}
	s.st.leader, s.st.polled = &l, now
}

// Follow records that this replica follows the leader l, or, if it's nil,
// that it knows no leader. The election calls it at each poll, and when the
// replica steps down, once Run has returned.
func (s *Service) Follow(l *store.Leader) {
	if l != nil {
		copied := *l
		l = &copied
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.leads, s.st.leader, s.st.polled = false, l, s.o.now()
	// A follower schedules no refresh.
	s.st.next = time.Time{}
}

// NextRefresh returns when the next scheduled refresh starts, or zero while
// one runs, or if none is scheduled, for the leader's row.
func (s *Service) NextRefresh() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.st.lastStart.After(s.st.lastEnd) {
		return time.Time{}
	}
	return s.st.next
}

// viewsOf returns the views that groups serve.
func viewsOf(groups []Group) map[string]bool {
	views := map[string]bool{}
	for _, g := range groups {
		for _, v := range g.Views {
			views[v] = true
		}
	}
	return views
}

// SetGroups changes the groups that the service compares, as `nbpdns
// groups` and the config file's mirror change them (ADR-0040). /status and
// the API show them at once, and webhooks' zones are matched with their
// views. A removed group's state goes, with every metric series of its
// name. A renamed group, matched by its ID, keeps its state under its new
// name, and its series move there (ADR-0045). A new group that reuses a
// removed one's name, with another ID, starts with no state. Their clients
// are the caller's, and a refresh's results for a group that's gone are
// dropped.
func (s *Service) SetGroups(groups []Group) {
	s.mu.Lock()
	oldName, oldID := map[int64]string{}, map[string]int64{}
	for _, g := range s.groups {
		if g.ID != 0 {
			oldName[g.ID] = g.Name
		}
		oldID[g.Name] = g.ID
	}
	// renamed maps an old name to its new one; kept are the names that
	// stay, with the same group.
	renamed, kept := map[string]string{}, map[string]bool{}
	for _, g := range groups {
		if old, ok := oldName[g.ID]; g.ID != 0 && ok && old != g.Name {
			renamed[old] = g.Name
		} else if id, ok := oldID[g.Name]; ok && id == g.ID {
			kept[g.Name] = true
		}
	}
	var dropped []string
	states := map[string]*groupState{}
	moved := map[string]groupState{}
	for _, g := range s.groups {
		gs := s.st.groups[g.Name]
		switch to := renamed[g.Name]; {
		case to != "":
			dropped = append(dropped, g.Name)
			if gs != nil {
				gs.report.Group = to
				states[to], moved[to] = gs, *gs
			}
		case kept[g.Name]:
			if gs != nil {
				states[g.Name] = gs
			}
		default:
			dropped = append(dropped, g.Name)
		}
	}
	s.st.groups, s.groups, s.views = states, groups, viewsOf(groups)
	s.mu.Unlock()
	for _, name := range dropped {
		s.o.Metrics.DropGroup(name)
	}
	for name, gs := range moved {
		publishState(s.o.Metrics, name, gs)
	}
}

// RefreshNow brings the next full refresh forward to now, as the
// schedule's, so that a group that's new or changed is compared within
// seconds. A refresh that's running is followed by another.
func (s *Service) RefreshNow() {
	s.mu.Lock()
	s.st.refreshNow = true
	if st := &s.st; !st.next.IsZero() && !st.lastStart.After(st.lastEnd) {
		st.next = s.o.now()
	}
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// configured reports whether the service compares a group named name. It's
// called with s.mu held.
func (s *Service) configured(name string) bool {
	return slices.ContainsFunc(s.groups, func(g Group) bool { return g.Name == name })
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
// overlap: one that takes longer than the interval delays the next. It
// first loads what the history holds, which a replica that has just taken
// over may not have loaded yet (ADR-0048), so that its zone refreshes merge
// into the last reports.
func (s *Service) Run(ctx context.Context) {
	if err := s.Reload(ctx); err != nil && ctx.Err() == nil {
		s.o.Log.WarnContext(ctx, "couldn't load the last drift reports from the database before refreshing; the first refresh replaces them",
			"err", err)
	}
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
		// The schedule is as precise as the history, which the leader's row
		// gives the followers its next refresh in.
		start := s.kept(s.o.now())
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
// if it's zero, or RefreshNow brought it forward, or until what webhooks
// queued is due, whichever is sooner, and takes what's queued. full reports
// whether a full refresh is due: the scheduled one, which covers every zone
// queued, or one that a webhook asked for. A wake, from a webhook or a
// change of timing or of the groups, works the time out again.
func (s *Service) wait(ctx context.Context, nextFn func() time.Time) (b batch, full bool) {
	for {
		now := s.o.now()
		next := nextFn()
		s.mu.Lock()
		if s.st.refreshNow {
			next = now
		}
		until := next
		if p := &s.st.pending; p.events > 0 {
			until = minTime(until, p.due(s.Timing().WebhookDelay, s.o.maxWait))
		}
		s.mu.Unlock()
		if !until.After(now) {
			b := s.take()
			full := !now.Before(next) || b.full
			if full {
				// This refresh covers what RefreshNow asked for.
				s.mu.Lock()
				s.st.refreshNow = false
				s.mu.Unlock()
			}
			return b, full
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
// and serves it once it's recorded (ADR-0048). If webhooks queued b, the
// refresh covers it: its trace
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
	res := s.found(rctx, log, r, err, start, end)
	cause := store.CauseSchedule
	if b.full {
		cause = store.CauseWebhook
	}
	if !s.commit(rctx, log, res, store.RefreshRecord{Kind: store.RefreshFull, Cause: cause, Started: start, Finished: end,
		Outcome: res.outcome, Error: errText(err), NetBoxRequestIDs: b.requestIDs()}) {
		return
	}
	if b.events > 0 {
		s.noteWebhookRefresh(b, true, start, end, res.outcome, err)
	}
}

// A result is what a refresh found, which the service serves once the
// history has it (ADR-0048).
type result struct {
	// full is set for a full refresh, and clear for a zone refresh.
	full       bool
	start, end time.Time
	outcome    string
	// err is why the refresh failed: NetBox's error, or the timeout's.
	err error
	// netbox is store.NetBoxUp or store.NetBoxDown, as the refresh found
	// NetBox, or empty if it says nothing of NetBox, as a refresh that ran
	// out of time, or compared no group, doesn't.
	netbox string
	// zones are NetBox's zones that the refresh read, to keep, or nil.
	zones *store.NetBoxZones
	// changes are what it found of each group that it read, or tried to.
	changes []change
}

// commit writes the refresh, rec, which found res, to the history, if
// there's one, and serves it once it's written (ADR-0048). It reports
// whether it served it: a refresh that can't be written isn't served, by
// this replica or any other, and keep counts it as a write failure.
func (s *Service) commit(ctx context.Context, log *slog.Logger, res result, rec store.RefreshRecord) bool {
	var version int64
	if s.o.History != nil {
		var ok bool
		if version, ok = s.keep(ctx, log, res, rec); !ok {
			return false
		}
	}
	s.serve(res, version)
	return true
}

// keep writes a refresh, rec, with what it found, res, to the history, and
// returns the history's version once it has it. A failed write is counted,
// logged and shown on /status, and reported false: the refresh isn't served,
// and the next full refresh covers what it found.
func (s *Service) keep(ctx context.Context, log *slog.Logger, res result, rec store.RefreshRecord) (int64, bool) {
	rec.TraceID = trace.SpanContextFromContext(ctx).TraceID().String()
	for _, c := range res.changes {
		g := c.g
		if c.failed {
			rec.Groups = append(rec.Groups, store.GroupResult{Group: g.Group, Status: drift.StatusFailed, Error: g.Error})
			continue
		}
		rec.Groups = append(rec.Groups, store.GroupResult{Group: g.Group, Status: drift.StatusOK, Counts: g.Counts})
		rec.Reports = append(rec.Reports, store.KeptReport{Report: g, LastSuccess: c.lastSuccess})
	}
	rec.NetBoxStatus, rec.NetBoxZones = res.netbox, res.zones
	if res.netbox == store.NetBoxDown {
		rec.NetBoxError = errText(res.err)
	}
	// The refresh's own timeout may have passed.
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), historyWriteTimeout)
	defer cancel()
	w, err := s.o.History.RecordRefresh(wctx, rec)
	if err != nil {
		s.o.Metrics.DBWriteFailure(metrics.WriteHistory)
		s.mu.Lock()
		s.st.writeFailures++
		s.st.lastWriteError, s.st.lastWriteFailure = err.Error(), s.o.now()
		s.mu.Unlock()
		log.ErrorContext(wctx, "couldn't write the refresh to the drift history, so no replica serves it; the next full refresh covers what it found",
			"err", err)
		return 0, false
	}
	s.o.Metrics.AuditEvents(w.Events)
	log.DebugContext(wctx, "wrote the refresh to the drift history", "refresh_id", w.ID, "zone_rows", w.ZoneRows,
		"netbox_zone_rows", w.NetBoxZones, "audit_events", len(w.Events))
	return w.ID, true
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

// found returns what a full refresh found, r, or err, from start to end, as
// the service would keep it, without keeping it yet; counts the refresh,
// which this replica ran; and logs what it found to log. A group that
// couldn't be read keeps its last report, and if NetBox couldn't be read,
// or the refresh ran out of time, every group does.
func (s *Service) found(ctx context.Context, log *slog.Logger, r drift.Report, err error, start, end time.Time) result {
	m := s.o.Metrics
	m.Refreshes.WithLabelValues(outcomeOf(r, err)).Inc()
	m.RefreshDuration.Observe(end.Sub(start).Seconds())
	start, end = s.kept(start), s.kept(end)
	res := result{full: true, start: start, end: end, outcome: outcomeOf(r, err), err: err}
	s.mu.Lock()
	s.st.refreshes[res.outcome]++
	s.mu.Unlock()

	var te *TimeoutError
	if errors.As(err, &te) {
		log.WarnContext(ctx, "the drift refresh took longer than drift.timeout, so it was stopped, and every server group keeps its last report",
			"timeout_seconds", te.Timeout.Seconds())
		return res
	}
	if err != nil {
		res.netbox = store.NetBoxDown
		log.WarnContext(ctx, "the drift refresh couldn't read NetBox, so every server group keeps its last report", "err", err)
		return res
	}
	// With no groups to compare, as when every group is yet to be added
	// (ADR-0040), the refresh read nothing, so NetBox's state stays as it
	// was.
	if len(r.Groups) > 0 {
		res.netbox = store.NetBoxUp
	}
	if r.NetBox != nil {
		res.zones = &store.NetBoxZones{Zones: r.NetBox}
	}
	if r.NetBoxErr != nil {
		log.WarnContext(ctx, "couldn't read the records of NetBox's zones that aren't compared, so the API keeps NetBox's last records",
			"err", r.NetBoxErr)
	}
	s.mu.Lock()
	res.changes = s.changes(r.Groups, end, false)
	s.mu.Unlock()
	logChanges(ctx, log, res.changes)
	return res
}

// kept returns t as the history keeps it, if there's one, so that what this
// replica serves of its refreshes is what the others load (ADR-0048).
func (s *Service) kept(t time.Time) time.Time {
	if s.o.History == nil {
		return t
	}
	return store.Stored(t)
}

// serve keeps what a refresh found, res, which the history has as of
// version, or which no history keeps, for 0, so that the service serves it,
// and sets the drift metrics from it.
func (s *Service) serve(res result, version int64) {
	m := s.o.Metrics
	s.mu.Lock()
	defer s.mu.Unlock()
	st := &s.st
	st.version = max(st.version, version)
	if res.full {
		st.ready, st.lastEndStart, st.lastEnd, st.lastOutcome, st.lastError = true, res.start, res.end, res.outcome, errText(res.err)
		m.LastRefresh.WithLabelValues().Set(unix(res.end))
		if res.outcome == metrics.OutcomeComplete {
			st.lastComplete = res.end
			m.LastCompleteRefresh.WithLabelValues().Set(unix(res.end))
		}
	}
	switch res.netbox {
	case store.NetBoxUp:
		st.netboxRead, st.netboxUp, st.netboxError = true, true, ""
	case store.NetBoxDown:
		st.netboxRead, st.netboxUp, st.netboxError = true, false, errText(res.err)
	}
	if res.netbox != "" {
		m.NetBoxUp.WithLabelValues().Set(gauge(st.netboxUp))
	}
	switch z := res.zones; {
	case z == nil:
	case z.Scope == nil:
		st.netbox = netboxView(z.Zones, res.end)
	default:
		st.netbox = mergeNetBox(st.netbox, z.Zones, z.Scope)
	}
	setChanges(m, s.applyChanges(res.changes))
}

// A change is what a refresh found for a group, for keep to write, and for
// applyChanges to keep once it's written.
type change struct {
	// g is the report to keep: for a zone refresh, the group's last report
	// with the zone refresh's merged in.
	g      drift.GroupReport
	failed bool
	// now and removed are the drifted-zone series that keeping it sets and
	// removes, which applyChanges works out.
	now     map[[2]string]bool
	removed [][2]string
	// zones are those that a zone refresh compared, or nil.
	zones []string
	// lastSuccess is the group's last full comparison's time.
	lastSuccess time.Time
}

// changes returns what a refresh found of groups, read at end, as each
// group would keep it: a group that failed keeps its last report. With
// merge, groups are a zone refresh's, which are merged into the last
// reports; a group with no report waits for the next full refresh. Groups
// that the service doesn't compare are left out. It's called with s.mu
// held, and changes nothing.
func (s *Service) changes(groups []drift.GroupReport, end time.Time, merge bool) []change {
	var out []change
	for _, g := range groups {
		if !s.configured(g.Group) {
			continue
		}
		gs := s.st.groups[g.Group]
		if merge && gs == nil {
			continue
		}
		if g.Status != drift.StatusOK {
			out = append(out, change{g: g, failed: true})
			continue
		}
		c := change{g: g, zones: g.Compared, lastSuccess: end}
		if merge {
			// The last success is the last full comparison's: a zone
			// refresh compares only some zones, and the rest of the report
			// is as old.
			if gs.lastSuccess.IsZero() {
				continue
			}
			c.g, c.lastSuccess = drift.Merge(gs.report, g), gs.lastSuccess
		}
		out = append(out, c)
	}
	return out
}

// applyChanges keeps each group's change, and returns those kept, with the
// drifted-zone series that each sets and removes. A group that SetGroups
// removed, or renamed, since the refresh started is dropped, as the history
// drops it. It's called with s.mu held.
func (s *Service) applyChanges(changes []change) []change {
	var out []change
	for _, c := range changes {
		name := c.g.Group
		if !s.configured(name) {
			continue
		}
		gs := s.st.groups[name]
		if gs == nil {
			gs = &groupState{drifted: map[[2]string]bool{}}
			s.st.groups[name] = gs
		}
		if c.failed {
			gs.up, gs.lastError = false, c.g.Error
			out = append(out, c)
			continue
		}
		gs.up, gs.lastError, gs.report, gs.lastSuccess = true, "", c.g, c.lastSuccess
		c.now, c.removed = drifted(c.g), nil
		for k := range gs.drifted {
			if !c.now[k] {
				c.removed = append(c.removed, k)
			}
		}
		gs.drifted = c.now
		out = append(out, c)
	}
	return out
}

// setChanges sets each kept group's metrics.
func setChanges(m *metrics.Metrics, changes []change) {
	for _, c := range changes {
		g := c.g
		if c.failed {
			m.GroupUp.WithLabelValues(g.Group).Set(0)
			continue
		}
		m.GroupUp.WithLabelValues(g.Group).Set(1)
		m.GroupLastSuccess.WithLabelValues(g.Group).Set(unix(c.lastSuccess))
		setGroup(m, g, c.now, c.removed)
	}
}

// logChanges logs what a refresh found of each group to log. A zone refresh
// logs only its own zones' warnings and drift.
func logChanges(ctx context.Context, log *slog.Logger, changes []change) {
	for _, c := range changes {
		g := c.g
		if c.failed {
			log.WarnContext(ctx, "a server group couldn't be read, so it keeps its last report", "group", g.Group, "err", g.Error)
			continue
		}
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

// livez answers 200 unless the service is stuck: on the leader, its
// refreshes; on a follower, its election.
func (s *Service) livez(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	alive, since := s.alive(&s.st)
	leads := s.st.leads
	s.mu.Unlock()
	switch {
	case alive:
		text(w, http.StatusOK, "ok")
	case leads:
		text(w, http.StatusServiceUnavailable, fmt.Sprintf("stuck: no drift refresh has started since %s", since.UTC().Format(time.RFC3339)))
	default:
		text(w, http.StatusServiceUnavailable, fmt.Sprintf("stuck: this follower hasn't tried for the leader's lock since %s", since.UTC().Format(time.RFC3339)))
	}
}

// alive reports whether the service isn't stuck, from st, and since when it
// has made no progress. The leader is alive if a refresh has started within
// the time a refresh can take, since the last one started, or since it took
// over or the service started if none has since; a restored refresh from
// before the start doesn't count. A follower is alive if its election has
// polled within electionStall, or the service started within it. It's
// called with s.mu held.
func (s *Service) alive(st *state) (bool, time.Time) {
	if !st.leads {
		last := st.polled
		if last.Before(s.started) {
			last = s.started
		}
		return s.o.now().Sub(last) <= electionStall, last
	}
	last := st.lastStart
	if last.Before(s.started) {
		last = s.started
	}
	if last.Before(st.leadSince) {
		last = st.leadSince
	}
	t := s.Timing()
	return s.o.now().Sub(last) <= t.Interval+t.Timeout+time.Minute, last
}

// readyz answers 200 once the service serves a full refresh that finished,
// whatever its outcome (ADR-0029): its own, once it's recorded, or one that
// it loaded from the history, at start or from the leader (ADR-0042,
// ADR-0048).
func (s *Service) readyz(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	ready, leads, failures := s.st.ready, s.st.leads, s.st.writeFailures
	s.mu.Unlock()
	switch {
	case ready:
		text(w, http.StatusOK, "ready")
	case !leads:
		text(w, http.StatusServiceUnavailable, "not ready: the database holds no drift refresh yet; this follower is ready once the leader records its first")
	case failures > 0:
		text(w, http.StatusServiceUnavailable, "not ready: no drift refresh could be written to the database yet, so none is served; /status shows why")
	default:
		text(w, http.StatusServiceUnavailable, "not ready: the first drift refresh hasn't finished, and the database holds none from before")
	}
}

func text(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = fmt.Fprintln(w, body)
}
