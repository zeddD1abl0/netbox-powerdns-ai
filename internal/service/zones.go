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
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/webhook"
)

// The bounds of the zone refreshes that NetBox's webhooks ask for
// (ADR-0035).
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

// A batch is what NetBox's webhooks asked to refresh, gathered until its
// refresh runs.
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

// noteWebhookRefresh keeps b's refresh, which finished, as the last one
// that webhooks asked for. full marks a full refresh, which b may not have
// asked for: a scheduled one that covered b's zones.
func (s *Service) noteWebhookRefresh(b batch, full bool, start, end time.Time, outcome string, err error) {
	w := &WebhookRefresh{Started: start.UTC(), Finished: end.UTC(), Full: full, Reason: b.reason,
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
	s.mu.Lock()
	s.st.lastWebhookRefresh = w
	s.mu.Unlock()
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

// Notify queues the refresh that a NetBox event, e, asks for, r (ADR-0035),
// and reports whether it queued anything: not if r asks for nothing, or
// names only zones, or a view, that no group serves. It implements
// api.Notifier. ctx carries the webhook's span, which the refresh links
// to.
func (s *Service) Notify(ctx context.Context, e webhook.Event, r webhook.Refresh) bool {
	var zones []drift.ZoneRef
	for _, z := range r.Zones {
		if s.views[z.View] {
			zones = append(zones, drift.ZoneRef{View: z.View, Name: z.Name})
		}
	}
	now := s.o.now()
	s.mu.Lock()
	s.st.lastEvent = &Event{Received: now.UTC(), Event: e.Event, ObjectType: e.ObjectType, Request: Request{ID: e.RequestID(), User: e.User()}}
	s.mu.Unlock()
	if !r.Full && len(zones) == 0 {
		return false
	}
	// A view that no group serves changes nothing that nbpdns compares.
	if r.Views != nil && !slices.ContainsFunc(r.Views, func(v string) bool { return s.views[v] }) {
		return false
	}
	s.mu.Lock()
	b := &s.st.pending
	if b.events == 0 {
		b.first, b.zones = now, map[drift.ZoneRef]bool{}
	}
	b.events++
	b.last = now
	if r.Full && !b.full {
		b.full, b.reason = true, r.Reason
	}
	for _, z := range zones {
		if b.full {
			break
		}
		b.zones[z] = true
		if len(b.zones) > maxZones {
			b.full, b.reason = true, fmt.Sprintf("more than %d zones changed", maxZones)
		}
	}
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() && len(b.links) < maxRequests {
		b.links = append(b.links, trace.Link{SpanContext: sc})
	}
	if id := e.RequestID(); id != "" && len(b.requests) < maxRequests &&
		!slices.ContainsFunc(b.requests, func(r Request) bool { return r.ID == id }) {
		b.requests = append(b.requests, Request{ID: id, User: e.User()})
	}
	pending := len(b.zones)
	s.mu.Unlock()
	s.o.Metrics.PendingZones.Set(float64(pending))
	// The loop may be waiting for the next scheduled refresh.
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return true
}

// take returns what's waiting for a refresh, and empties the queue.
func (s *Service) take() batch {
	s.mu.Lock()
	b := s.st.pending
	s.st.pending = batch{}
	s.mu.Unlock()
	s.o.Metrics.PendingZones.Set(0)
	return b
}

// refreshZones runs b's zone refresh, as its own trace, linked to the
// webhooks that asked for it, and records it. A refresh cut short by ctx's
// cancellation isn't recorded.
func (s *Service) refreshZones(ctx context.Context, start time.Time, b batch) {
	log := s.o.Log.With("netbox_request_ids", b.requestIDs())
	rctx := logging.WithRequestID(ctx, rand.Text())
	rctx, span := s.o.Tracer.Start(rctx, "drift zone refresh", trace.WithNewRoot(),
		trace.WithLinks(b.links...), trace.WithAttributes(b.attributes()...))
	defer span.End()
	rctx, cancel := context.WithTimeout(rctx, s.o.Timeout)
	defer cancel()
	log.InfoContext(rctx, "refreshing the zones that NetBox's webhooks named", "zones", b.names(),
		"events", b.events, "netbox_users", b.users())
	r, err := s.o.Refresh(rctx, b.refs())
	if ctx.Err() != nil {
		return
	}
	if errors.Is(rctx.Err(), context.DeadlineExceeded) && (err != nil || !r.Complete) {
		err = &TimeoutError{Timeout: s.o.Timeout}
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	end := s.o.now()
	outcome := s.recordZones(rctx, log, r, err, start, end)
	s.noteWebhookRefresh(b, false, start, end, outcome, err)
}

// recordZones keeps what a zone refresh found, merged into the last-known
// state, sets the metrics, and logs it, as record does for a full refresh,
// and returns its outcome. A zone refresh merges only into what a full
// refresh has kept: a group with no report, or NetBox's records before
// NetBox was first read, wait for the next full refresh.
func (s *Service) recordZones(ctx context.Context, log *slog.Logger, r drift.Report, err error, start, end time.Time) string {
	m := s.o.Metrics
	outcome := outcomeOf(r, err)
	m.ZoneRefreshes.WithLabelValues(outcome).Inc()
	m.ZoneRefreshDuration.Observe(end.Sub(start).Seconds())
	var te *TimeoutError
	if errors.As(err, &te) {
		log.WarnContext(ctx, "the zone refresh took longer than drift.timeout, so it was stopped; its zones wait for the next scheduled refresh",
			"timeout_seconds", te.Timeout.Seconds())
		return outcome
	}
	if err != nil {
		s.mu.Lock()
		s.st.netboxUp, s.st.netboxError = false, err.Error()
		s.mu.Unlock()
		m.NetBoxUp.WithLabelValues().Set(0)
		log.WarnContext(ctx, "the zone refresh couldn't read NetBox; its zones wait for the next scheduled refresh", "err", err)
		return outcome
	}
	s.mu.Lock()
	st := &s.st
	st.netboxUp, st.netboxError = true, ""
	switch {
	case r.NetBoxErr != nil:
		log.WarnContext(ctx, "couldn't read the records of NetBox's zones that aren't compared, so the API keeps NetBox's last records",
			"err", r.NetBoxErr)
	case r.NetBox != nil && r.Scope != nil && !st.netbox.AsOf.IsZero():
		st.netbox = mergeNetBox(st.netbox, r.NetBox, r.Scope)
	}
	changes := s.apply(r.Groups, end, true)
	s.mu.Unlock()
	m.NetBoxUp.WithLabelValues().Set(1)
	s.publish(ctx, log, changes, end)
	return outcome
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
