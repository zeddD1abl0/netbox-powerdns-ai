package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store/sqlite"
)

// verifyPage is how many events VerifyAudit reads at a time.
const verifyPage = 1000

// A Tx is a transaction on the store: a change, and the audit events that
// record it, which commit or roll back together (ADR-0039).
type Tx struct {
	q sqlite.Querier
	// appended are the events appended, for InTx's caller once it commits.
	appended []audit.Event
}

// InTx runs fn in a write transaction, which it commits if fn returns nil,
// and rolls back otherwise. Write transactions run one at a time, on either
// database. It returns the audit events that fn appended, once they're
// committed.
func (s *Store) InTx(ctx context.Context, fn func(tx *Tx) error) ([]audit.Event, error) {
	sqlTx, err := s.db.BeginTx(ctx, s.dialect.write)
	if err != nil {
		return nil, fmt.Errorf("starting a transaction: %w", err)
	}
	defer func() { _ = sqlTx.Rollback() }() // after Commit, it does nothing
	if s.dialect.lockWrites != nil {
		if err := s.dialect.lockWrites(ctx, sqlTx); err != nil {
			return nil, fmt.Errorf("waiting for the other writes: %w", err)
		}
	}
	tx := &Tx{q: s.dialect.queries(sqlTx)}
	if err := fn(tx); err != nil {
		return nil, err
	}
	if err := sqlTx.Commit(); err != nil {
		return nil, fmt.Errorf("committing a transaction: %w", err)
	}
	return tx.appended, nil
}

// Audit appends e to the trail, after its last event, and links it to that
// event. It sets e's Seq, PrevHash and Hash, its Outcome to Success if it has
// none, and its Time to now if it has none, and returns it. It fails on an
// action that isn't declared, or a target of another type than its action's.
func (t *Tx) Audit(ctx context.Context, e audit.Event) (audit.Event, error) {
	def, ok := audit.Lookup(e.Action)
	switch {
	case !ok:
		return e, fmt.Errorf("recording an audit event: the action %q isn't declared", e.Action)
	case e.Target.Type != def.Target:
		return e, fmt.Errorf("recording an audit event: %s's target is a %s, not a %s", e.Action, def.Target, e.Target.Type)
	case e.Before != nil && !json.Valid(e.Before), e.After != nil && !json.Valid(e.After):
		return e, fmt.Errorf("recording an audit event: %s's before or after isn't JSON", e.Action)
	}
	if e.Outcome == "" {
		e.Outcome = audit.Success
	}
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	e.Time = e.Time.UTC().Truncate(time.Microsecond)
	last, err := t.q.LastAuditEvent(ctx)
	switch {
	case err == nil:
		e.Seq, e.PrevHash = last.Seq+1, last.Hash
	case errors.Is(err, sql.ErrNoRows):
		// The trail is empty: it starts afresh, or after its anchor.
		a, err := t.q.GetAuditAnchor(ctx)
		switch {
		case err == nil:
			e.Seq, e.PrevHash = a.Seq+1, a.Hash
		case errors.Is(err, sql.ErrNoRows):
			e.Seq, e.PrevHash = 1, ""
		default:
			return e, fmt.Errorf("recording an audit event: %w", err)
		}
	default:
		return e, fmt.Errorf("recording an audit event: %w", err)
	}
	e.Hash = e.Sum()
	err = t.q.InsertAuditEvent(ctx, sqlite.InsertAuditEventParams{
		Seq: e.Seq, At: e.Time.Format(TimeLayout), Actor: e.Actor.String(), Action: string(e.Action),
		TargetType: e.Target.Type, TargetID: e.Target.ID, BeforeJson: nullJSON(e.Before), AfterJson: nullJSON(e.After),
		Reason: e.Reason, Outcome: string(e.Outcome), TraceID: e.TraceID, RequestID: e.RequestID,
		SourceIp: e.SourceIP, PrevHash: e.PrevHash, Hash: e.Hash,
	})
	if err != nil {
		return e, fmt.Errorf("recording an audit event: %w", err)
	}
	t.appended = append(t.appended, e)
	return e, nil
}

// Audit appends e to the trail in a transaction of its own, for an event
// that records no change of the store's, such as serve's start.
func (s *Store) Audit(ctx context.Context, e audit.Event) (audit.Event, error) {
	var out audit.Event
	_, err := s.InTx(ctx, func(tx *Tx) error {
		var err error
		out, err = tx.Audit(ctx, e)
		return err
	})
	return out, err
}

// AuditFilter selects events. Its zero value selects every event.
type AuditFilter struct {
	// Since and Until bound the events' times: from Since, and before
	// Until. Zero leaves that end open.
	Since, Until time.Time
	Action       audit.Action
	Actor        string
	// TargetType and TargetID select the events about a target, or, with
	// only TargetType, about targets of a type.
	TargetType, TargetID string
	// Limit is the most events to return, the latest. Zero means 100.
	Limit int
}

// ListAudit returns the latest events that f selects, oldest first.
func (s *Store) ListAudit(ctx context.Context, f AuditFilter) ([]audit.Event, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	timeArg := func(t time.Time) sql.NullString {
		if t.IsZero() {
			return sql.NullString{}
		}
		return textArg(t.UTC().Format(TimeLayout))
	}
	rows, err := s.q.ListAuditEvents(ctx, sqlite.ListAuditEventsParams{
		Since: timeArg(f.Since), Until: timeArg(f.Until), Action: textArg(string(f.Action)), Actor: textArg(f.Actor),
		TargetType: textArg(f.TargetType), TargetID: textArg(f.TargetID), MaxEvents: int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("reading the audit trail: %w", err)
	}
	events := make([]audit.Event, len(rows))
	for i, r := range rows {
		if events[i], err = eventOf(r); err != nil {
			return nil, err
		}
	}
	slices.Reverse(events)
	return events, nil
}

// AuditSummary is what VerifyAudit found.
type AuditSummary struct {
	// Count is how many events the trail holds.
	Count int
	// First and Last are its first and last events, if it has any.
	First, Last audit.Event
	// Anchor is where it starts, if a prune has deleted its oldest events.
	Anchor *audit.Anchor
}

// VerifyAudit walks the whole trail, in order, in one read transaction, and
// checks each event's hash and its link to the event before it. It returns a
// *audit.BrokenError where the chain breaks.
func (s *Store) VerifyAudit(ctx context.Context) (AuditSummary, error) {
	sqlTx, q, err := s.readTx(ctx)
	if err != nil {
		return AuditSummary{}, fmt.Errorf("starting a transaction: %w", err)
	}
	defer func() { _ = sqlTx.Rollback() }()
	var sum AuditSummary
	switch a, err := q.GetAuditAnchor(ctx); {
	case err == nil:
		sum.Anchor = &audit.Anchor{Seq: a.Seq, Hash: a.Hash}
	case !errors.Is(err, sql.ErrNoRows):
		return sum, fmt.Errorf("reading the audit trail's anchor: %w", err)
	}
	v := audit.NewVerifier(sum.Anchor)
	var after int64
	for {
		rows, err := q.AuditEventsAfter(ctx, sqlite.AuditEventsAfterParams{After: after, MaxEvents: verifyPage})
		if err != nil {
			return sum, fmt.Errorf("reading the audit trail: %w", err)
		}
		for _, r := range rows {
			e, err := eventOf(r)
			if err != nil {
				return sum, err
			}
			if err := v.Check(e); err != nil {
				return sum, err
			}
			if v.Count() == 1 {
				sum.First = e
			}
			sum.Last, after = e, e.Seq
		}
		if len(rows) < verifyPage {
			sum.Count = v.Count()
			return sum, nil
		}
	}
}

// PruneAudit deletes the events before cutoff, from the trail's start up to
// the first one at or after it, so that none younger than cutoff goes,
// keeps the last one it deletes as the trail's anchor, and records the
// prune as an event, all in one transaction. It returns how many it deleted, and the prune's event,
// if it deleted any.
func (s *Store) PruneAudit(ctx context.Context, cutoff time.Time, retentionDays int) (int64, []audit.Event, error) {
	var deleted int64
	events, err := s.InTx(ctx, func(tx *Tx) error {
		last, err := tx.q.LastAuditEventBefore(ctx, cutoff.UTC().Format(TimeLayout))
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		first, err := tx.q.FirstAuditEvent(ctx)
		if err != nil {
			return err
		}
		if deleted, err = tx.q.DeleteAuditEventsThrough(ctx, last.Seq); err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := tx.q.SetAuditAnchor(ctx, sqlite.SetAuditAnchorParams{
			Seq: last.Seq, Hash: last.Hash, PrunedAt: now.Format(TimeLayout),
		}); err != nil {
			return err
		}
		before, err := audit.Value(map[string]int64{"first_seq": first})
		if err != nil {
			return err
		}
		after, err := audit.Value(map[string]any{
			"deleted": deleted, "anchor_seq": last.Seq, "anchor_hash": last.Hash, "retention_days": retentionDays,
		})
		if err != nil {
			return err
		}
		_, err = tx.Audit(ctx, audit.Event{
			Time: now, Actor: audit.System(), Action: audit.AuditPruned, Target: audit.Target{Type: "audit", ID: "trail"},
			Before: before, After: after,
		}.WithContext(ctx))
		return err
	})
	if err != nil {
		return 0, nil, fmt.Errorf("pruning the audit trail: %w", err)
	}
	return deleted, events, nil
}

// eventOf returns the event that a row holds.
func eventOf(r sqlite.AuditEvent) (audit.Event, error) {
	t, err := time.Parse(TimeLayout, r.At)
	if err != nil {
		return audit.Event{}, fmt.Errorf("reading audit event %d: its time: %w", r.Seq, err)
	}
	jsonOf := func(n sql.NullString) json.RawMessage {
		if !n.Valid {
			return nil
		}
		return json.RawMessage(n.String)
	}
	return audit.Event{
		Seq: r.Seq, Time: t, Actor: audit.ParseActor(r.Actor), Action: audit.Action(r.Action),
		Target: audit.Target{Type: r.TargetType, ID: r.TargetID}, Before: jsonOf(r.BeforeJson), After: jsonOf(r.AfterJson),
		Reason: r.Reason, Outcome: audit.Outcome(r.Outcome), TraceID: r.TraceID, RequestID: r.RequestID,
		SourceIP: r.SourceIp, PrevHash: r.PrevHash, Hash: r.Hash,
	}, nil
}

// nullJSON returns j for a nullable column.
func nullJSON(j json.RawMessage) sql.NullString {
	return sql.NullString{String: string(j), Valid: j != nil}
}

// textArg returns s for an optional filter: NULL, which selects everything,
// if it's empty.
func textArg(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
