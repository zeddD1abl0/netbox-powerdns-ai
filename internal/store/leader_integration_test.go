//go:build integration

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/lab"
)

// replicas opens a new, migrated PostgreSQL database, and returns a store on
// it for each of n replicas, as n processes would open it, and its URL.
func replicas(t *testing.T, n int) ([]*Store, string) {
	t.Helper()
	where := lab.NewPostgresDatabase(t)
	stores := make([]*Store, n)
	for i := range stores {
		stores[i] = postgresBackend.connect(t, where)
	}
	if _, err := stores[0].Migrate(t.Context(), "test"); err != nil {
		t.Fatal(err)
	}
	return stores, where
}

// join joins s's database as replica name, and ends the session when the
// test ends.
func join(t *testing.T, s *Store, name string) *Session {
	t.Helper()
	ss, err := s.Join(t.Context(), Replica{Name: name, ID: strings.ToUpper(name) + "-ID"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ss.Close)
	return ss
}

// TestSessionSettings checks that a replica's session bounds how long
// PostgreSQL keeps its locks once it's lost (ADR-0047), and names it.
func TestSessionSettings(t *testing.T) {
	stores, _ := replicas(t, 1)
	ss := join(t, stores[0], "a")
	// PostgreSQL shows tcp_user_timeout in milliseconds, without a unit.
	want := map[string]string{
		"tcp_keepalives_idle": "10", "tcp_keepalives_interval": "5", "tcp_keepalives_count": "3",
		"tcp_user_timeout": "25000", "idle_session_timeout": "30s", "application_name": holderName(),
	}
	for name, value := range want {
		var got string
		if err := ss.conn.QueryRowContext(t.Context(), "SELECT current_setting($1)", name).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != value {
			t.Errorf("the session's %s = %q, want %q", name, got, value)
		}
	}
	// The pool's sessions keep PostgreSQL's defaults.
	var idle string
	if err := stores[0].db.QueryRowContext(t.Context(), "SELECT current_setting('idle_session_timeout')").Scan(&idle); err != nil || idle != "0" {
		t.Errorf("the pool's idle_session_timeout = %q, %v, want 0", idle, err)
	}
}

// TestReplicasShareTheServeLock checks that replicas join together, that the
// command line's lock refuses while any of them runs, naming them, and that
// none joins while the command line holds it.
func TestReplicasShareTheServeLock(t *testing.T) {
	stores, _ := replicas(t, 3)
	a := join(t, stores[0], "a")
	join(t, stores[1], "b")

	_, err := stores[2].Lock(t.Context())
	var le *LockedError
	if !errors.As(err, &le) {
		t.Fatalf("Lock() while two replicas run: error = %v, want a LockedError", err)
	}
	holder := strings.TrimPrefix(holderName(), "nbpdns: ") + ", since "
	if le.Holders != 2 || strings.Count(le.Holder, holder) != 2 ||
		!strings.HasPrefix(err.Error(), "2 other nbpdns processes hold the database's lock: "+holder) {
		t.Errorf("Lock() while two replicas run: %+v: %v", le, err)
	}

	a.Close()
	a.Close() // a second Close does nothing
	// Only b holds it now.
	if _, err := stores[2].Lock(t.Context()); !errors.As(err, &le) || le.Holders != 1 {
		t.Errorf("Lock() while one replica runs: error = %v", err)
	}
}

// TestNoReplicaJoinsWhileLocked checks that a replica refuses to start while
// the command line, such as db migrate, holds the serve lock.
func TestNoReplicaJoinsWhileLocked(t *testing.T) {
	stores, _ := replicas(t, 2)
	l, err := stores[0].Lock(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	_, err = stores[1].Join(t.Context(), Replica{Name: "a", ID: "A"})
	var le *LockedError
	if !errors.As(err, &le) || le.Holders != 1 || !strings.Contains(err.Error(), "another nbpdns holds the database's lock") {
		t.Errorf("Join() while the command line holds the lock: error = %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	join(t, stores[1], "a")
}

// TestElection checks that one replica at a time takes the leader's lock,
// that its takeover is recorded, with the leader before, and that another
// takes it once the leader's session ends.
func TestElection(t *testing.T) {
	stores, _ := replicas(t, 2)
	a, b := join(t, stores[0], "a"), join(t, stores[1], "b")
	if l, err := stores[1].Leader(t.Context()); err != nil || l != nil {
		t.Fatalf("Leader() before an election = %+v, %v, want none", l, err)
	}

	next := time.Now().Add(time.Minute).UTC().Truncate(time.Microsecond)
	ta, err := a.TryLead(t.Context(), next)
	if err != nil || ta == nil {
		t.Fatalf("a's TryLead() = %+v, %v, want a takeover", ta, err)
	}
	if ta.Previous != nil || ta.Leader.Replica != a.me || !ta.Leader.NextRefresh.Equal(next) || len(ta.Events) != 1 {
		t.Errorf("a's takeover = %+v", ta)
	}
	e := ta.Events[0]
	if e.Action != audit.ServiceLeaderAcquired || e.Target != (audit.Target{Type: "service", ID: "a"}) || e.Before != nil ||
		string(e.After) != `{"instance_id":"A-ID","replica":"a"}` {
		t.Errorf("a's takeover's event = %+v, before %s, after %s", e, e.Before, e.After)
	}
	if tb, err := b.TryLead(t.Context(), time.Time{}); err != nil || tb != nil {
		t.Fatalf("b's TryLead() while a leads = %+v, %v, want nothing", tb, err)
	}
	l, err := stores[1].Leader(t.Context())
	if err != nil || l == nil || l.Replica != a.me || !l.Since.Equal(ta.Leader.Since) || !l.NextRefresh.Equal(next) {
		t.Fatalf("Leader() on b = %+v, %v, want a's row", l, err)
	}

	seen, err := a.Heartbeat(t.Context(), time.Time{})
	if err != nil || !seen.After(l.LastSeen) {
		t.Fatalf("a's Heartbeat() = %v, %v, want a time after %v", seen, err, l.LastSeen)
	}
	if l, err := stores[1].Leader(t.Context()); err != nil || !l.LastSeen.Equal(seen) || !l.NextRefresh.IsZero() {
		t.Errorf("Leader() after the heartbeat = %+v, %v", l, err)
	}

	// The leader stops: its session ends, and b takes over.
	a.Close()
	tb, err := b.TryLead(t.Context(), time.Time{})
	if err != nil || tb == nil {
		t.Fatalf("b's TryLead() after a stopped = %+v, %v, want a takeover", tb, err)
	}
	if p := tb.Previous; p == nil || p.Replica != a.me || !p.LastSeen.Equal(seen) {
		t.Errorf("b's takeover's previous leader = %+v, want a, last seen %v", p, seen)
	}
	var before map[string]any
	if err := json.Unmarshal(tb.Events[0].Before, &before); err != nil {
		t.Fatal(err)
	}
	if before["replica"] != "a" || before["instance_id"] != "A-ID" || before["last_seen"] != seen.Format(time.RFC3339Nano) {
		t.Errorf("b's takeover's before = %s", tb.Events[0].Before)
	}
	events, err := stores[1].ListAudit(t.Context(), AuditFilter{Action: audit.ServiceLeaderAcquired})
	if err != nil || len(events) != 2 {
		t.Errorf("ListAudit(service.leader_acquired) = %d events, %v, want 2", len(events), err)
	}
	if _, err := a.Heartbeat(t.Context(), time.Time{}); err == nil {
		t.Error("the stopped leader's Heartbeat() succeeded")
	}
}

// TestFence checks that leader-only writes fail on a replica that doesn't
// lead, and on a leader whose lock is gone, though it hasn't noticed
// (ADR-0047).
func TestFence(t *testing.T) {
	stores, where := replicas(t, 3)
	a, b := join(t, stores[0], "a"), join(t, stores[1], "b")
	if ta, err := a.TryLead(t.Context(), time.Time{}); err != nil || ta == nil {
		t.Fatalf("a's TryLead() = %+v, %v", ta, err)
	}
	writes := map[string]func(s *Store) error{
		"RecordRefresh": func(s *Store) error {
			_, err := s.RecordRefresh(t.Context(), full(time.Now()))
			return err
		},
		"PruneDriftHistory": func(s *Store) error {
			_, _, err := s.PruneDriftHistory(t.Context(), time.Now())
			return err
		},
		"PruneAudit": func(s *Store) error {
			_, _, err := s.PruneAudit(t.Context(), time.Now(), 1)
			return err
		},
	}
	check := func(who string, s *Store, want error) {
		t.Helper()
		for name, write := range writes {
			if err := write(s); !errors.Is(err, want) {
				t.Errorf("%s by %s: error = %v, want %v", name, who, err, want)
			}
		}
	}
	check("the leader", stores[0], nil)
	check("a follower", stores[1], ErrNotLeader)
	// The command line's store joined nothing, and writes freely.
	check("the command line", stores[2], nil)

	// Any replica queues a webhook's refresh (ADR-0048), but only the
	// leader's refresh covers it.
	q := &QueuedRefresh{Received: time.Now(), Zones: []drift.ZoneRef{{View: "v", Name: "a.example."}}}
	if err := stores[1].RecordWebhook(t.Context(), WebhookEvent{Received: q.Received, Event: "updated", ObjectType: "netbox_dns.record"}, q); err != nil {
		t.Fatalf("a follower's RecordWebhook() = %v", err)
	}
	covering := full(time.Now())
	covering.Queue = &CoveredQueue{Through: q.ID}
	if _, err := stores[1].RecordRefresh(t.Context(), covering); !errors.Is(err, ErrNotLeader) {
		t.Errorf("a follower's RecordRefresh() of the queue: error = %v, want ErrNotLeader", err)
	}
	queued := func() int {
		t.Helper()
		w, err := stores[0].LoadWebhooks(t.Context(), -1, 0)
		if err != nil {
			t.Fatal(err)
		}
		return len(w.Queued)
	}
	if n := queued(); n != 1 {
		t.Errorf("after the follower's refresh, %d rows queued, want 1", n)
	}
	if _, err := stores[0].RecordRefresh(t.Context(), covering); err != nil {
		t.Errorf("the leader's RecordRefresh() of the queue = %v", err)
	}
	if n := queued(); n != 0 {
		t.Errorf("after the leader's refresh, %d rows queued, want none", n)
	}

	// PostgreSQL ends the leader's session, as it does a lost leader's,
	// and b takes over. a hasn't noticed, but can't write.
	admin, err := sql.Open("pgx", where)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err := admin.ExecContext(t.Context(), "SELECT pg_terminate_backend($1)", a.pid); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		tb, err := b.TryLead(t.Context(), time.Time{})
		return err == nil && tb != nil
	})
	check("the deposed leader", stores[0], ErrNotLeader)
	check("the new leader", stores[1], nil)
	if _, err := a.Heartbeat(t.Context(), time.Time{}); err == nil {
		t.Error("the deposed leader's Heartbeat() succeeded")
	}
}

// waitFor waits up to 10 seconds for cond.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for !cond() {
		select {
		case <-ctx.Done():
			t.Fatal("timed out")
		case <-time.After(50 * time.Millisecond):
		}
	}
}
