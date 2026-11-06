package service

import (
	"cmp"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/logging"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/metrics"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/webhook"
)

// The bounds of the zone refreshes that NetBox's webhooks ask for
// (ADR-0036).
const (
	// maxWait is the longest that webhooks' zones wait for their refresh,
	// from the first webhook, unless Options.maxWait sets another.
	maxWait = 30 * time.Second
	// maxZones is the most zones a zone refresh compares. More make a full
	// refresh instead.
	maxZones = 100
	// maxRequests bounds the webhook spans that a refresh links to, and the
	// NetBox requests that it records.
	maxRequests = 128
)

// A batch is what NetBox's webhooks asked to refresh, gathered from the
// queue's rows, in order (ADR-0036, ADR-0048).
type batch struct {
	// events counts the webhooks gathered: the first came at first, and the
	// last at last.
	events      int
	first, last time.Time
	zones       map[drift.ZoneRef]bool
	// full asks for a full refresh, for reason.
	full   bool
	reason string
	// links are the webhooks' spans, and requests the NetBox requests they
	// told of, each once, up to maxRequests.
	links    []trace.Link
	requests []Request
	// through is the last row gathered.
	through int64
}

// add gathers the queue's row r, which comes after every row that b holds.
// More than maxZones zones make a full refresh instead.
func (b *batch) add(r store.QueuedRefresh) {
	if b.events == 0 {
		b.first, b.last, b.zones = r.Received, r.Received, map[drift.ZoneRef]bool{}
	}
	b.events++
	// The replicas' clocks may differ a little, and a row's ID is its
	// commit's place, not its webhook's.
	b.first, b.last, b.through = minTime(b.first, r.Received), maxTime(b.last, r.Received), r.ID
	if r.Full && !b.full {
		b.full, b.reason = true, r.Reason
	}
	for _, z := range r.Zones {
		if b.full {
			break
		}
		b.zones[z] = true
		if len(b.zones) > maxZones {
			b.full, b.reason = true, fmt.Sprintf("more than %d zones changed", maxZones)
		}
	}
	if l, ok := linkOf(r); ok && len(b.links) < maxRequests {
		b.links = append(b.links, l)
	}
	if id := r.RequestID; id != "" && len(b.requests) < maxRequests &&
		!slices.ContainsFunc(b.requests, func(q Request) bool { return q.ID == id }) {
		b.requests = append(b.requests, Request{ID: id, User: r.User})
	}
}

// clone returns a copy of b that shares nothing with it.
func (b batch) clone() batch {
	b.zones, b.links, b.requests = maps.Clone(b.zones), slices.Clone(b.links), slices.Clone(b.requests)
	return b
}

// linkOf returns the link to the span of r's webhook, if it had one.
func linkOf(r store.QueuedRefresh) (trace.Link, bool) {
	tid, err := trace.TraceIDFromHex(r.TraceID)
	if err != nil {
		return trace.Link{}, false
	}
	sid, err := trace.SpanIDFromHex(r.SpanID)
	if err != nil {
		return trace.Link{}, false
	}
	return trace.Link{SpanContext: trace.NewSpanContext(trace.SpanContextConfig{TraceID: tid, SpanID: sid, Remote: true})}, true
}

// hooks are what NetBox's webhooks left, as the database holds them, or,
// without a history, as this replica took them (ADR-0048). They're guarded
// by Service.mu.
type hooks struct {
	// version is the webhooks' version in the history that they're as of.
	version int64
	// covered is the queue's mark, the last row that a recorded refresh
	// covered, or -1 until they're gathered; seen is the last row gathered.
	covered, seen int64
	// queue gathers the rows after the mark, which /status shows, and which
	// the leader's next refresh takes, whole. pending gathers those after
	// taken, the last row that this replica's refreshes took, whose delay
	// says when the next zone refresh is due: rows up to it that are still
	// queued, as they are after a refresh that couldn't be written, wait
	// for a later webhook, or the schedule, rather than make another
	// refresh at once.
	queue, pending batch
	taken          int64
	// lastEvent is the last webhook's event, and lastRefresh the last
	// refresh that webhooks asked for, or nil.
	lastEvent   *Event
	lastRefresh *WebhookRefresh
}

// add gathers the queue's row r, the next after seen.
func (h *hooks) add(r store.QueuedRefresh) {
	h.queue.add(r)
	if r.ID > h.taken {
		h.pending.add(r)
	}
	h.seen = r.ID
}

// load makes w, which the history holds, the webhooks' state. If the
// queue's mark has moved, it gathers the queue again, from w's rows, which
// are then every row after the mark; otherwise, it gathers w's rows, which
// come after seen.
func (h *hooks) load(w store.WebhookState) {
	h.version = w.Version
	h.lastEvent, h.lastRefresh = nil, nil
	if e := w.LastEvent; e != nil {
		h.lastEvent = &Event{Received: e.Received, Event: e.Event, ObjectType: e.ObjectType, Request: Request{ID: e.RequestID, User: e.User}}
	}
	if r := w.LastRefresh; r != nil {
		h.lastRefresh = &WebhookRefresh{Started: r.Started, Finished: r.Finished, Zones: r.Zones, Full: r.Full, Reason: r.Reason,
			Outcome: r.Outcome, Error: r.Error, Events: r.Events, Requests: []Request{}}
		for _, q := range r.Requests {
			h.lastRefresh.Requests = append(h.lastRefresh.Requests, Request(q))
		}
	}
	if w.Covered != h.covered {
		h.covered, h.seen, h.taken = w.Covered, w.Covered, max(h.taken, w.Covered)
		h.queue, h.pending = batch{}, batch{}
	}
	for _, r := range w.Queued {
		h.add(r)
	}
}

// retake has the leader's next refresh take every row queued, such as
// those that a refresh that this replica didn't record took, as when it
// stepped down.
func (h *hooks) retake() {
	h.pending, h.taken = h.queue.clone(), h.covered
}

// A Request is a NetBox request whose changes a webhook told of.
type Request struct {
	ID   string `json:"id"`
	User string `json:"user"`
}

// An Event is a NetBox webhook's event that the service was told of.
type Event struct {
	Received   time.Time `json:"received"`
	Event      string    `json:"event"`
	ObjectType string    `json:"object_type"`
	// Request is the NetBox request that made the change: its ID and user
	// are empty if no request did.
	Request Request `json:"request"`
}

// A WebhookRefresh is a refresh that NetBox's webhooks asked for, or that
// covered the zones they named, which finished.
type WebhookRefresh struct {
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished"`
	// Zones are the zones refreshed, each as view/name, or nil for a full
	// refresh, which Full marks, with its reason.
	Zones  []string `json:"zones"`
	Full   bool     `json:"full"`
	Reason string   `json:"reason"`
	// Outcome is complete, incomplete or failed.
	Outcome  string    `json:"outcome"`
	Error    string    `json:"error"`
	Events   int       `json:"events"`
	Requests []Request `json:"requests"`
}

// webhookRefresh returns b's refresh, which finished, as the webhooks' last
// refresh, as the history keeps it. full marks a full refresh, which b may
// not have asked for: a scheduled one that covered b's zones.
func (s *Service) webhookRefresh(b batch, full bool, start, end time.Time, outcome string, err error) *WebhookRefresh {
	w := &WebhookRefresh{Started: s.kept(start), Finished: s.kept(end), Full: full, Reason: b.reason,
		Outcome: outcome, Events: b.events, Requests: slices.Clone(b.requests)}
	switch {
	case !full:
		w.Zones = b.names()
	case !b.full:
		w.Reason = "the scheduled refresh came first"
	}
	if err != nil {
		w.Error = err.Error()
	}
	if w.Requests == nil {
		w.Requests = []Request{}
	}
	return w
}

// coveredQueue returns what a refresh of b, w, covered of the queue, for the
// history to record, or nil if b is empty.
func coveredQueue(b batch, w *WebhookRefresh) *store.CoveredQueue {
	if b.events == 0 {
		return nil
	}
	c := &store.CoveredQueue{Through: b.through, Refresh: store.WebhookRefresh{Started: w.Started, Finished: w.Finished,
		Zones: w.Zones, Full: w.Full, Reason: w.Reason, Outcome: w.Outcome, Error: w.Error, Events: w.Events}}
	for _, r := range w.Requests {
		c.Refresh.Requests = append(c.Refresh.Requests, store.WebhookRequest(r))
	}
	return c
}

// cover serves what a refresh of b, w, which is recorded, covered of the
// queue: the rows up to b's last go, and w is the webhooks' last refresh.
func (s *Service) cover(b batch, w *WebhookRefresh) {
	if b.events == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	h := &s.st.hooks
	h.lastRefresh = w
	// What came since the refresh took b is what's left. Otherwise, the
	// queue's mark moved meanwhile, and the next reload gathers it again.
	if h.taken == b.through {
		h.covered, h.queue = b.through, h.pending.clone()
	}
	s.setPending()
}

// setPending sets the pending zones' gauge from the queue. It's called with
// s.mu held.
func (s *Service) setPending() {
	s.o.Metrics.PendingZones.Set(float64(len(s.st.hooks.queue.zones)))
}

// due returns when b's refresh is due: once no webhook has come for delay,
// or longest after the first, whichever is sooner.
func (b *batch) due(delay, longest time.Duration) time.Time {
	return minTime(b.last.Add(delay), b.first.Add(longest))
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// refs returns b's zones, sorted by view and name.
func (b *batch) refs() []drift.ZoneRef {
	return slices.SortedFunc(maps.Keys(b.zones), func(x, y drift.ZoneRef) int {
		return cmp.Or(strings.Compare(x.View, y.View), dns.CompareNames(x.Name, y.Name))
	})
}

// names returns the names of b's zones, as the logs and spans show them:
// each in its view, as view/name. They're never nil.
func (b *batch) names() []string {
	out := []string{}
	for _, z := range b.refs() {
		out = append(out, z.View+"/"+z.Name)
	}
	return out
}

// requestIDs and users return the IDs and users of b's NetBox requests.
func (b *batch) requestIDs() []string {
	out := make([]string, len(b.requests))
	for i, r := range b.requests {
		out[i] = r.ID
	}
	return out
}

func (b *batch) users() []string {
	var out []string
	for _, r := range b.requests {
		if r.User != "" && !slices.Contains(out, r.User) {
			out = append(out, r.User)
		}
	}
	return out
}

// attributes are those of the span of b's refresh (Q-037).
func (b *batch) attributes() []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.Int("netbox.events", b.events),
		attribute.StringSlice("netbox.request_ids", b.requestIDs()),
		attribute.StringSlice("netbox.users", b.users()),
	}
	if b.full {
		return append(attrs, attribute.String("nbpdns.full_refresh_reason", b.reason))
	}
	return append(attrs, attribute.StringSlice("nbpdns.zones", b.names()))
}

// Notify queues the refresh that a NetBox event, e, asks for, r (ADR-0036),
// and reports whether it queued anything: not if r asks for nothing, or
// names only zones, or a view, that no group serves. Either way, e is the
// last event. With a history, it writes them there, for whichever replica
// leads to refresh (ADR-0048), and its error says that it couldn't. It
// implements api.Notifier. ctx carries the webhook's span, which the
// refresh links to.
func (s *Service) Notify(ctx context.Context, e webhook.Event, r webhook.Refresh) (bool, error) {
	now := s.kept(s.o.now())
	ev := store.WebhookEvent{Received: now, Event: e.Event, ObjectType: e.ObjectType, RequestID: e.RequestID(), User: e.User()}
	s.mu.Lock()
	views := s.views
	s.mu.Unlock()
	q := queued(ctx, now, e, r, views)
	if s.o.History != nil {
		if err := s.o.History.RecordWebhook(ctx, ev, q); err != nil {
			s.o.Metrics.DBWriteFailure(metrics.WriteWebhook)
			return false, err
		}
		// This replica shows it at once, and gathers it if it leads.
		if err := s.reloadWebhooks(ctx); err != nil && ctx.Err() == nil {
			s.o.Log.WarnContext(ctx, "queued a webhook's refresh, but couldn't load the queue back from the database; "+
				"serve loads it at its next check", "err", err)
		}
	} else {
		s.mu.Lock()
		h := &s.st.hooks
		h.lastEvent = &Event{Received: now, Event: ev.Event, ObjectType: ev.ObjectType, Request: Request{ID: ev.RequestID, User: ev.User}}
		if q != nil {
			q.ID = h.seen + 1
			h.add(*q)
			s.setPending()
		}
		s.mu.Unlock()
	}
	if q == nil {
		return false, nil
	}
	s.poke()
	return true, nil
}

// queued returns the refresh that a NetBox event, e, received at now, asks
// for, r, to queue: of the zones it names, those in views, the views that
// the groups serve. It's nil if r asks for nothing, or names only zones, or
// a view, that no group serves. ctx carries the webhook's span.
func queued(ctx context.Context, now time.Time, e webhook.Event, r webhook.Refresh, views map[string]bool) *store.QueuedRefresh {
	var zones []drift.ZoneRef
	for _, z := range r.Zones {
		if views[z.View] {
			zones = append(zones, drift.ZoneRef{View: z.View, Name: z.Name})
		}
	}
	if !r.Full && len(zones) == 0 {
		return nil
	}
	// A view that no group serves changes nothing that nbpdns compares.
	if r.Views != nil && !slices.ContainsFunc(r.Views, func(v string) bool { return views[v] }) {
		return nil
	}
	q := &store.QueuedRefresh{Received: now, Full: r.Full, Reason: r.Reason, RequestID: e.RequestID(), User: e.User()}
	if !r.Full {
		q.Zones = zones
	}
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		q.TraceID, q.SpanID = sc.TraceID().String(), sc.SpanID().String()
	}
	return q
}

// poke tells Run to work out when its next refresh is due.
func (s *Service) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// take returns what the leader's next refresh covers of the queue, every
// row queued, and reports whether it's a full refresh: the scheduled one,
// if scheduled is set, or one that the queue asks for.
func (s *Service) take(scheduled bool) (batch, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := &s.st.hooks
	b := h.queue.clone()
	h.pending, h.taken = batch{}, h.seen
	return b, scheduled || b.full
}

// refreshZones runs b's zone refresh, as its own trace, linked to the
// webhooks that asked for it, and serves it once it's recorded (ADR-0048).
// A refresh cut short by ctx's cancellation isn't recorded.
func (s *Service) refreshZones(ctx context.Context, start time.Time, b batch) {
	log := s.o.Log.With("netbox_request_ids", b.requestIDs())
	rctx := logging.WithRequestID(ctx, rand.Text())
	rctx, span := s.o.Tracer.Start(rctx, "drift zone refresh", trace.WithNewRoot(),
		trace.WithLinks(b.links...), trace.WithAttributes(b.attributes()...))
	defer span.End()
	timeout := s.Timing().Timeout
	rctx, cancel := context.WithTimeout(rctx, timeout)
	defer cancel()
	log.InfoContext(rctx, "refreshing the zones that NetBox's webhooks named", "zones", b.names(),
		"events", b.events, "netbox_users", b.users())
	r, err := s.o.Refresh(rctx, b.refs())
	if ctx.Err() != nil {
		return
	}
	if errors.Is(rctx.Err(), context.DeadlineExceeded) && (err != nil || !r.Complete) {
		err = &TimeoutError{Timeout: timeout}
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	end := s.o.now()
	res := s.foundZones(rctx, log, r, err, start, end)
	w := s.webhookRefresh(b, false, start, end, res.outcome, err)
	if !s.commit(rctx, log, res, store.RefreshRecord{Kind: store.RefreshZones, Cause: store.CauseWebhook, Started: start, Finished: end,
		Outcome: res.outcome, Error: errText(err), Zones: b.names(), NetBoxRequestIDs: b.requestIDs(), Queue: coveredQueue(b, w)}) {
		return
	}
	s.cover(b, w)
}

// foundZones returns what a zone refresh found, r, or err, merged into the
// last-known state, as the service would keep it, without keeping it yet,
// as found does for a full refresh; counts it; and logs what it found. A
// zone refresh merges only into what a full refresh has kept: a group with
// no report, or NetBox's records before NetBox was first read, wait for the
// next full refresh.
func (s *Service) foundZones(ctx context.Context, log *slog.Logger, r drift.Report, err error, start, end time.Time) result {
	m := s.o.Metrics
	m.ZoneRefreshes.WithLabelValues(outcomeOf(r, err)).Inc()
	m.ZoneRefreshDuration.Observe(end.Sub(start).Seconds())
	start, end = s.kept(start), s.kept(end)
	res := result{start: start, end: end, outcome: outcomeOf(r, err), err: err}
	var te *TimeoutError
	if errors.As(err, &te) {
		log.WarnContext(ctx, "the zone refresh took longer than drift.timeout, so it was stopped; its zones wait for the next scheduled refresh",
			"timeout_seconds", te.Timeout.Seconds())
		return res
	}
	if err != nil {
		res.netbox = store.NetBoxDown
		log.WarnContext(ctx, "the zone refresh couldn't read NetBox; its zones wait for the next scheduled refresh", "err", err)
		return res
	}
	// With no group serving its zones, the zone refresh read nothing.
	if len(r.Groups) > 0 {
		res.netbox = store.NetBoxUp
	}
	s.mu.Lock()
	if r.NetBoxErr == nil && r.NetBox != nil && r.Scope != nil && !s.st.netbox.AsOf.IsZero() {
		res.zones = &store.NetBoxZones{Zones: r.NetBox, Scope: r.Scope}
	}
	res.changes = s.changes(r.Groups, end, true)
	s.mu.Unlock()
	if r.NetBoxErr != nil {
		log.WarnContext(ctx, "couldn't read the records of NetBox's zones that aren't compared, so the API keeps NetBox's last records",
			"err", r.NetBoxErr)
	}
	logChanges(ctx, log, res.changes)
	return res
}

// outcomeOf returns a refresh's outcome, one of metrics.Outcomes.
func outcomeOf(r drift.Report, err error) string {
	switch {
	case err != nil:
		return metrics.OutcomeFailed
	case !r.Complete:
		return metrics.OutcomeIncomplete
	}
	return metrics.OutcomeComplete
}

// mergeNetBox returns old with a zone refresh's NetBox zones, zones, which
// it listed for sc: in each of sc's views, the zones with sc's names are
// those in zones, and no others. old isn't changed, nor any map it shares
// with what the API holds.
func mergeNetBox(old NetBoxView, zones []dns.Zone, sc *drift.Scope) NetBoxView {
	out := NetBoxView{AsOf: old.AsOf, Zones: maps.Clone(old.Zones)}
	if out.Zones == nil {
		out.Zones = map[string]map[string]dns.Zone{}
	}
	for _, v := range sc.Views {
		in := maps.Clone(out.Zones[v])
		if in == nil {
			in = map[string]dns.Zone{}
		}
		for _, n := range sc.Names {
			delete(in, n)
		}
		for _, z := range zones {
			if z.View == v {
				in[z.Name] = z
			}
		}
		if len(in) == 0 {
			delete(out.Zones, v)
			continue
		}
		out.Zones[v] = in
	}
	return out
}
