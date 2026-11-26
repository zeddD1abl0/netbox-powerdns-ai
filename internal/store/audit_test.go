package store

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
)

// started returns a service.started event at time at, by host.
func started(at time.Time, host string) audit.Event {
	return audit.Event{Time: at, Actor: audit.System(), Action: audit.ServiceStarted,
		Target: audit.Target{Type: "service", ID: host}, After: json.RawMessage(`{"version":"test"}`)}
}

// appendAll appends events, each in its own transaction.
func appendAll(t *testing.T, s *Store, events ...audit.Event) []audit.Event {
	t.Helper()
	out := make([]audit.Event, len(events))
	for i, e := range events {
		var err error
		if out[i], err = s.Audit(t.Context(), e); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestAuditAppends(t *testing.T) {
	onEach(t, func(t *testing.T, db backend) {
		s := db.migrated(t)
		t0 := time.Date(2026, 10, 1, 0, 0, 0, 123456789, time.UTC)
		es := appendAll(t, s, started(t0, "a"), started(t0.Add(time.Hour), "b"))
		if es[0].Seq != 1 || es[0].PrevHash != "" || es[1].Seq != 2 || es[1].PrevHash != es[0].Hash {
			t.Errorf("appended %+v, want 1 then 2, linked", es)
		}
		if es[0].Outcome != audit.Success || !es[0].Time.Equal(t0.Truncate(time.Microsecond)) {
			t.Errorf("appended %+v, want success, at %v to the microsecond", es[0], t0)
		}
		got, err := s.ListAudit(t.Context(), AuditFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].Hash != es[0].Hash || got[1].Hash != es[1].Hash || got[0].Sum() != got[0].Hash {
			t.Errorf("ListAudit() = %+v, want the two events as appended", got)
		}
	})
}

// TestAuditCommitsWithTheChange checks that an event and the transaction it's
// in commit or roll back together.
func TestAuditCommitsWithTheChange(t *testing.T) {
	onEach(t, func(t *testing.T, db backend) {
		s := db.migrated(t)
		ctx := t.Context()
		boom := errors.New("the change failed")
		_, err := s.InTx(ctx, func(tx *Tx) error {
			if _, err := tx.Audit(ctx, started(time.Now(), "a")); err != nil {
				return err
			}
			return boom
		})
		if !errors.Is(err, boom) {
			t.Fatalf("InTx() = %v, want %v", err, boom)
		}
		got, err := s.ListAudit(ctx, AuditFilter{})
		if err != nil || len(got) != 0 {
			t.Errorf("after a rollback, ListAudit() = %+v, %v, want nothing", got, err)
		}
		committed, err := s.InTx(ctx, func(tx *Tx) error {
			_, err := tx.Audit(ctx, started(time.Now(), "a"))
			return err
		})
		if err != nil || len(committed) != 1 || committed[0].Seq != 1 {
			t.Errorf("InTx() = %+v, %v, want the one event, at 1", committed, err)
		}
	})
}

func TestAuditRefuses(t *testing.T) {
	onEach(t, func(t *testing.T, db backend) {
		s := db.migrated(t)
		tests := []struct {
			name string
			e    audit.Event
			want string
		}{
			{"an undeclared action", audit.Event{Action: "zone.deleted", Target: audit.Target{Type: "zone"}},
				`the action "zone.deleted" isn't declared`},
			{"a target of another type", audit.Event{Action: audit.ServiceStarted, Target: audit.Target{Type: "zone"}},
				"service.started's target is a service, not a zone"},
			{"before that isn't JSON", audit.Event{Action: audit.ServiceStarted, Target: audit.Target{Type: "service"},
				Before: json.RawMessage("{")}, "before or after isn't JSON"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := s.Audit(t.Context(), tt.e)
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Errorf("Audit() = %v, want %q", err, tt.want)
				}
			})
		}
	})
}

func TestListAuditFilters(t *testing.T) {
	onEach(t, func(t *testing.T, db backend) {
		s := db.migrated(t)
		t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		stopped := audit.Event{Time: t0.Add(3 * time.Hour), Actor: audit.Actor{Kind: audit.ActorOSUser, ID: "jordan"},
			Action: audit.ServiceStopped, Target: audit.Target{Type: "service", ID: "b"}}
		appendAll(t, s, started(t0, "a"), started(t0.Add(time.Hour), "b"), started(t0.Add(2*time.Hour), "a"), stopped)
		tests := []struct {
			name string
			f    AuditFilter
			want []int64
		}{
			{"everything", AuditFilter{}, []int64{1, 2, 3, 4}},
			{"the latest two", AuditFilter{Limit: 2}, []int64{3, 4}},
			{"since", AuditFilter{Since: t0.Add(time.Hour)}, []int64{2, 3, 4}},
			{"until", AuditFilter{Until: t0.Add(time.Hour)}, []int64{1}},
			{"an action", AuditFilter{Action: audit.ServiceStopped}, []int64{4}},
			{"an actor", AuditFilter{Actor: "os-user:jordan"}, []int64{4}},
			{"a target", AuditFilter{TargetType: "service", TargetID: "a"}, []int64{1, 3}},
			{"a target type", AuditFilter{TargetType: "service"}, []int64{1, 2, 3, 4}},
			{"nothing matches", AuditFilter{TargetType: "zone"}, nil},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, err := s.ListAudit(t.Context(), tt.f)
				if err != nil {
					t.Fatal(err)
				}
				var seqs []int64
				for _, e := range got {
					seqs = append(seqs, e.Seq)
				}
				if !slices.Equal(seqs, tt.want) {
					t.Errorf("ListAudit(%+v) = %v, want %v", tt.f, seqs, tt.want)
				}
			})
		}
	})
}

// TestVerifyAudit breaks a trail in the database in each way it can be
// broken, and sees VerifyAudit find it.
func TestVerifyAudit(t *testing.T) {
	onEach(t, func(t *testing.T, db backend) {
		t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		tests := []struct {
			name    string
			tamper  func(ctx context.Context, s *Store) error
			wantSeq int64 // where it breaks, or 0 if it's intact
			want    string
		}{
			{"intact", func(context.Context, *Store) error { return nil }, 0, ""},
			{"an edited event", func(ctx context.Context, s *Store) error {
				_, err := s.db.ExecContext(ctx, "UPDATE audit_events SET reason = 'edited' WHERE seq = 3")
				return err
			}, 3, "its contents don't match its hash"},
			{"an edited event, its hash made again", func(ctx context.Context, s *Store) error {
				es, err := s.ListAudit(ctx, AuditFilter{})
				if err != nil {
					return err
				}
				e := es[2]
				e.Reason = "edited"
				_, err = s.db.ExecContext(ctx, "UPDATE audit_events SET reason = $1, hash = $2 WHERE seq = 3", e.Reason, e.Sum())
				return err
			}, 4, "doesn't link to the event before it"},
			{"a deleted event", func(ctx context.Context, s *Store) error {
				_, err := s.db.ExecContext(ctx, "DELETE FROM audit_events WHERE seq = 2")
				return err
			}, 3, "the events from 2 to 2 are missing"},
			{"the first event deleted", func(ctx context.Context, s *Store) error {
				_, err := s.db.ExecContext(ctx, "DELETE FROM audit_events WHERE seq = 1")
				return err
			}, 2, "the events from 1 to 1 are missing"},
			{"a forged anchor", func(ctx context.Context, s *Store) error {
				_, err := s.db.ExecContext(ctx, "INSERT INTO audit_anchor (singleton, seq, hash, pruned_at) VALUES (1, 0, 'forged', '')")
				return err
			}, 1, "doesn't link to the event before it"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				s := db.migrated(t)
				ctx := t.Context()
				var events []audit.Event
				for i := range 5 {
					events = append(events, started(t0.Add(time.Duration(i)*time.Hour), "a"))
				}
				appendAll(t, s, events...)
				if err := tt.tamper(ctx, s); err != nil {
					t.Fatal(err)
				}
				sum, err := s.VerifyAudit(ctx)
				if tt.wantSeq == 0 {
					if err != nil || sum.Count != 5 || sum.First.Seq != 1 || sum.Last.Seq != 5 || sum.Anchor != nil {
						t.Errorf("VerifyAudit() = %+v, %v, want 5 events, 1 to 5", sum, err)
					}
					return
				}
				var be *audit.BrokenError
				if !errors.As(err, &be) || be.Seq != tt.wantSeq || !strings.Contains(be.Problem, tt.want) {
					t.Errorf("VerifyAudit() = %v, want a break at %d with %q", err, tt.wantSeq, tt.want)
				}
			})
		}
	})
}

// TestVerifyAuditPages verifies a trail longer than a page.
func TestVerifyAuditPages(t *testing.T) {
	onEach(t, func(t *testing.T, db backend) {
		s := db.migrated(t)
		ctx := t.Context()
		n := verifyPage + 7
		_, err := s.InTx(ctx, func(tx *Tx) error {
			for i := range n {
				if _, err := tx.Audit(ctx, started(time.Now(), string(rune('a'+i%26)))); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		sum, err := s.VerifyAudit(ctx)
		if err != nil || sum.Count != n || sum.Last.Seq != int64(n) {
			t.Errorf("VerifyAudit() = %d events, last %d, %v, want %d", sum.Count, sum.Last.Seq, err, n)
		}
	})
}

func TestPruneAudit(t *testing.T) {
	onEach(t, func(t *testing.T, db backend) {
		s := db.migrated(t)
		ctx := t.Context()
		now := time.Now().UTC()
		old := now.AddDate(0, 0, -10)
		appended := appendAll(t, s, started(old, "a"), started(old.Add(time.Hour), "a"), started(now.Add(-time.Hour), "a"))

		deleted, events, err := s.PruneAudit(ctx, now.AddDate(0, 0, -7), 7)
		if err != nil {
			t.Fatal(err)
		}
		if deleted != 2 || len(events) != 1 || events[0].Action != audit.AuditPruned || events[0].Seq != 4 {
			t.Fatalf("PruneAudit() = %d, %+v, want 2 deleted and the prune's event, at 4", deleted, events)
		}
		var after map[string]any
		if err := json.Unmarshal(events[0].After, &after); err != nil {
			t.Fatal(err)
		}
		if after["deleted"] != 2.0 || after["anchor_seq"] != 2.0 || after["anchor_hash"] != appended[1].Hash || after["retention_days"] != 7.0 {
			t.Errorf("the prune's event's after = %v", after)
		}
		sum, err := s.VerifyAudit(ctx)
		if err != nil {
			t.Fatalf("VerifyAudit() after a prune: %v", err)
		}
		if sum.Count != 2 || sum.First.Seq != 3 || sum.Anchor == nil || *sum.Anchor != (audit.Anchor{Seq: 2, Hash: appended[1].Hash}) {
			t.Errorf("VerifyAudit() = %+v, want 3 and 4, after the anchor at 2", sum)
		}

		// Nothing more is old enough.
		deleted, events, err = s.PruneAudit(ctx, now.AddDate(0, 0, -7), 7)
		if err != nil || deleted != 0 || events != nil {
			t.Errorf("a second PruneAudit() = %d, %+v, %v, want nothing", deleted, events, err)
		}

		// The trail still breaks where it's tampered with.
		if _, err := s.db.ExecContext(ctx, "DELETE FROM audit_events WHERE seq = 3"); err != nil {
			t.Fatal(err)
		}
		var be *audit.BrokenError
		if _, err := s.VerifyAudit(ctx); !errors.As(err, &be) || be.Seq != 4 {
			t.Errorf("VerifyAudit() with the first event after the anchor deleted = %v, want a break at 4", err)
		}
	})
}

// TestPruneAuditKeepsAYoungerEvent prunes a trail in which an event follows
// one with a later time, as a refresh's events, which carry its end's time,
// follow a change that the command line recorded while the refresh was
// written. The prune stops at the first event that's young enough to keep.
func TestPruneAuditKeepsAYoungerEvent(t *testing.T) {
	onEach(t, func(t *testing.T, db backend) {
		s := db.migrated(t)
		cutoff := time.Now().UTC().AddDate(0, 0, -7)
		appended := appendAll(t, s, started(cutoff.Add(-time.Hour), "a"), started(cutoff.Add(time.Second), "a"),
			started(cutoff.Add(-time.Second), "a"))
		deleted, _, err := s.PruneAudit(t.Context(), cutoff, 7)
		if err != nil {
			t.Fatal(err)
		}
		sum, err := s.VerifyAudit(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if deleted != 1 || sum.First.Seq != 2 || sum.Anchor == nil || *sum.Anchor != (audit.Anchor{Seq: 1, Hash: appended[0].Hash}) {
			t.Errorf("PruneAudit() deleted %d, leaving %+v; want the first alone deleted, since the second is younger than the cutoff", deleted, sum)
		}
	})
}

// TestAuditAfterAnEmptyingPrune appends to a trail whose events were all
// deleted, but for its anchor, and sees the new event link to the anchor.
func TestAuditAfterAnEmptyingPrune(t *testing.T) {
	onEach(t, func(t *testing.T, db backend) {
		s := db.migrated(t)
		ctx := t.Context()
		es := appendAll(t, s, started(time.Now(), "a"))
		if _, err := s.db.ExecContext(ctx, "DELETE FROM audit_events"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.ExecContext(ctx, "INSERT INTO audit_anchor (singleton, seq, hash, pruned_at) VALUES (1, 1, $1, '')", es[0].Hash); err != nil {
			t.Fatal(err)
		}
		e := appendAll(t, s, started(time.Now(), "a"))[0]
		if e.Seq != 2 || e.PrevHash != es[0].Hash {
			t.Errorf("appended %+v, want 2, linked to the anchor", e)
		}
		if _, err := s.VerifyAudit(ctx); err != nil {
			t.Errorf("VerifyAudit() = %v", err)
		}
	})
}
