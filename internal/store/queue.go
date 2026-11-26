package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store/sqlite"
)

// seqQueue is the sequence that the queue's IDs come from, which is also the
// webhooks' version (ADR-0048): each webhook moves it, as does each refresh
// that covers the queue.
const seqQueue = "refresh_queue"

// A WebhookEvent is a NetBox webhook's event, as /status shows the last one.
type WebhookEvent struct {
	Received time.Time
	// Event is created, updated or deleted, and ObjectType the object's,
	// such as netbox_dns.record.
	Event, ObjectType string
	// RequestID and User are those of the NetBox request that made the
	// change, or empty if no request did.
	RequestID, User string
}

// A QueuedRefresh is a refresh that a NetBox webhook asked for, waiting in
// the queue for the leader to gather it (ADR-0048, ADR-0036).
type QueuedRefresh struct {
	// ID is its place in the queue, which RecordWebhook sets.
	ID       int64
	Received time.Time
	// Zones are the zones it names, in views that a server group serves,
	// unless Full is set: then it asks for a full refresh, for Reason.
	Zones  []drift.ZoneRef
	Full   bool
	Reason string
	// RequestID and User are those of the NetBox request that made the
	// change, or empty if no request did.
	RequestID, User string
	// TraceID and SpanID are those of the webhook's span, in hex, which the
	// refresh links to, or empty if it had none.
	TraceID, SpanID string
}

// A WebhookRequest is a NetBox request whose change a refresh covered.
type WebhookRequest struct {
	ID   string `json:"id"`
	User string `json:"user"`
}

// A WebhookRefresh is a refresh that NetBox's webhooks asked for, or that
// covered the zones they named, as the database keeps the last one, as
// JSON.
type WebhookRefresh struct {
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished"`
	// Zones are those refreshed, each as view/name, or nil for a full
	// refresh, which Full marks, with its reason.
	Zones  []string `json:"zones"`
	Full   bool     `json:"full"`
	Reason string   `json:"reason"`
	// Outcome is complete, incomplete or failed, and Error why it failed.
	Outcome string `json:"outcome"`
	Error   string `json:"error"`
	// Events counts the webhooks it served, and Requests are their NetBox
	// requests, each once.
	Events   int              `json:"events"`
	Requests []WebhookRequest `json:"requests"`
}

// CoveredQueue is what a refresh covered of the webhooks' queue
// (ADR-0048).
type CoveredQueue struct {
	// Through is the last row of the queue that the refresh gathered. Once
	// it's recorded, every row up to it goes.
	Through int64
	// Refresh is the refresh, as the webhooks' last refresh, which it
	// becomes.
	Refresh WebhookRefresh
}

// WebhookState is what NetBox's webhooks left in the database, for each
// replica to serve, and the leader to refresh (ADR-0048).
type WebhookState struct {
	// Version is the webhooks' version, as Versions reads it, that the state
	// is as of.
	Version int64
	// Covered is the queue's mark: the last row that a recorded refresh
	// covered. Every row up to it has gone.
	Covered int64
	// LastEvent is the last webhook's event, and LastRefresh the last
	// refresh that webhooks asked for, or that covered the zones they named.
	// Each is nil before the first.
	LastEvent   *WebhookEvent
	LastRefresh *WebhookRefresh
	// Queued are the queue's rows that LoadWebhooks was asked for, in order.
	Queued []QueuedRefresh
}

// queuedZone is a zone of a queued refresh, as zones_json holds it.
type queuedZone struct {
	View string `json:"view"`
	Name string `json:"name"`
}

// RecordWebhook records e as the last webhook's event, and, if q isn't nil,
// queues its refresh, q, whose ID it sets, in one transaction, which moves
// the webhooks' version (ADR-0048). Any replica can, so that a webhook to
// any of them is refreshed by the leader.
func (s *Store) RecordWebhook(ctx context.Context, e WebhookEvent, q *QueuedRefresh) error {
	var zones string
	if q != nil {
		z := make([]queuedZone, len(q.Zones))
		for i, r := range q.Zones {
			z[i] = queuedZone{View: r.View, Name: r.Name}
		}
		var err error
		if zones, err = jsonText(z); err != nil {
			return fmt.Errorf("queuing a webhook's refresh: %w", err)
		}
	}
	var id int64
	_, err := s.InTx(ctx, func(tx *Tx) error {
		var err error
		if id, err = tx.q.NextIDs(ctx, sqlite.NextIDsParams{N: 1, Name: seqQueue}); err != nil {
			return err
		}
		if err := tx.q.SetLastWebhookEvent(ctx, sqlite.SetLastWebhookEventParams{
			LastEventAt: e.Received.UTC().Format(TimeLayout), LastEvent: e.Event, LastObjectType: e.ObjectType,
			LastRequestID: e.RequestID, LastRequestUser: e.User,
		}); err != nil {
			return err
		}
		if q == nil {
			return nil
		}
		return tx.q.InsertQueuedRefresh(ctx, sqlite.InsertQueuedRefreshParams{
			ID: id, ReceivedAt: q.Received.UTC().Format(TimeLayout), ZonesJson: zones, FullRefresh: q.Full, Reason: q.Reason,
			RequestID: q.RequestID, RequestUser: q.User, TraceID: q.TraceID, SpanID: q.SpanID,
		})
	})
	if err != nil {
		if q != nil {
			return fmt.Errorf("queuing a webhook's refresh: %w", err)
		}
		return fmt.Errorf("recording a webhook's event: %w", err)
	}
	if q != nil {
		q.ID = id
	}
	return nil
}

// LoadWebhooks reads what NetBox's webhooks left, in one read transaction:
// the last event and refresh, the queue's mark, and the queue's rows after
// since, for a replica that holds those up to it as of the mark covered.
// If the mark has moved from covered, or is past since, it reads every row
// after the mark instead, for the replica to gather the queue again.
func (s *Store) LoadWebhooks(ctx context.Context, covered, since int64) (WebhookState, error) {
	var out WebhookState
	sqlTx, q, err := s.readTx(ctx)
	if err != nil {
		return out, fmt.Errorf("loading NetBox's webhooks: %w", err)
	}
	defer func() { _ = sqlTx.Rollback() }()
	fail := func(err error) (WebhookState, error) {
		return WebhookState{}, fmt.Errorf("loading NetBox's webhooks: %w", err)
	}

	if out.Version, err = q.SequenceValue(ctx, seqQueue); err != nil {
		return fail(err)
	}
	st, err := q.GetWebhookState(ctx)
	if err != nil {
		return fail(err)
	}
	out.Covered = st.CoveredThrough
	if st.LastEventAt != "" {
		at, err := time.Parse(TimeLayout, st.LastEventAt)
		if err != nil {
			return fail(err)
		}
		out.LastEvent = &WebhookEvent{Received: at, Event: st.LastEvent, ObjectType: st.LastObjectType,
			RequestID: st.LastRequestID, User: st.LastRequestUser}
	}
	if st.LastRefreshJson != "" {
		var r WebhookRefresh
		if err := json.Unmarshal([]byte(st.LastRefreshJson), &r); err != nil {
			return fail(fmt.Errorf("the last refresh: %w", err))
		}
		out.LastRefresh = &r
	}
	after := since
	if out.Covered != covered || since < out.Covered {
		after = out.Covered
	}
	rows, err := q.QueuedRefreshesAfter(ctx, after)
	if err != nil {
		return fail(err)
	}
	for _, r := range rows {
		qr := QueuedRefresh{ID: r.ID, Full: r.FullRefresh, Reason: r.Reason, RequestID: r.RequestID, User: r.RequestUser,
			TraceID: r.TraceID, SpanID: r.SpanID}
		if qr.Received, err = time.Parse(TimeLayout, r.ReceivedAt); err != nil {
			return fail(fmt.Errorf("queued refresh %d: %w", r.ID, err))
		}
		var zones []queuedZone
		if err := json.Unmarshal([]byte(r.ZonesJson), &zones); err != nil {
			return fail(fmt.Errorf("queued refresh %d's zones: %w", r.ID, err))
		}
		for _, z := range zones {
			qr.Zones = append(qr.Zones, drift.ZoneRef{View: z.View, Name: z.Name})
		}
		out.Queued = append(out.Queued, qr)
	}
	return out, nil
}

// coverQueue deletes the queue's rows that a refresh covered, c's, records
// the queue's mark and the webhooks' last refresh, and moves the webhooks'
// version, in t, the transaction that records the refresh.
func (t *Tx) coverQueue(ctx context.Context, c CoveredQueue) error {
	if _, err := t.q.DeleteQueuedRefreshesThrough(ctx, c.Through); err != nil {
		return err
	}
	r := c.Refresh
	r.Requests = nonNil(r.Requests)
	last, err := jsonText(r)
	if err != nil {
		return err
	}
	if err := t.q.SetQueueCovered(ctx, sqlite.SetQueueCoveredParams{CoveredThrough: c.Through, LastRefreshJson: last}); err != nil {
		return err
	}
	_, err = t.q.NextIDs(ctx, sqlite.NextIDsParams{N: 1, Name: seqQueue})
	return err
}
