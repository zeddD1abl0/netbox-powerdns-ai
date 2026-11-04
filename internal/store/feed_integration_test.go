//go:build integration

package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// listen listens for s's changes, and stops when the test ends.
func listen(t *testing.T, s *Store) *Listener {
	t.Helper()
	l, err := s.Listen(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Close)
	return l
}

// wait waits up to d for l to hear of a change, and returns Wait's error.
func wait(t *testing.T, l *Listener, d time.Duration) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), d)
	defer cancel()
	return l.Wait(ctx)
}

// TestNotify checks that every write transaction that commits notifies the
// replicas that listen, and one that rolls back doesn't (ADR-0048); and that
// a wait that times out leaves the listener working.
func TestNotify(t *testing.T) {
	stores, _ := replicas(t, 2)
	l := listen(t, stores[0])
	if err := wait(t, l, 200*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait() before any change = %v, want the deadline", err)
	}
	if _, err := stores[1].InTx(t.Context(), func(*Tx) error { return errors.New("no") }); err == nil {
		t.Fatal("InTx() of a failing change succeeded")
	}
	if err := wait(t, l, 200*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Wait() after a change that rolled back = %v, want the deadline", err)
	}
	if err := l.Check(t.Context()); err != nil {
		t.Errorf("Check() = %v", err)
	}
	if _, err := stores[1].Audit(t.Context(), started(time.Now(), "test")); err != nil {
		t.Fatal(err)
	}
	if err := wait(t, l, 5*time.Second); err != nil {
		t.Errorf("Wait() after a change = %v", err)
	}
	// One notification each: two changes, one of them while the listener
	// checks its connection, are heard twice, then no more.
	if _, err := stores[1].Audit(t.Context(), started(time.Now(), "test")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := l.Check(t.Context()); err != nil {
		t.Errorf("Check() = %v", err)
	}
	if _, err := stores[1].Audit(t.Context(), started(time.Now(), "test")); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		if err := wait(t, l, 5*time.Second); err != nil {
			t.Errorf("Wait() for change %d of two = %v", i+1, err)
		}
	}
	if err := wait(t, l, 200*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Wait() after both = %v, want the deadline", err)
	}
}

// TestListenerOutlastsIdleTimeout checks that the listener's connection,
// which waits idle, keeps listening where the database ends idle sessions,
// and is named as the replica's session is.
func TestListenerOutlastsIdleTimeout(t *testing.T) {
	stores, where := replicas(t, 1)
	var name string
	if err := stores[0].db.QueryRowContext(t.Context(), "SELECT current_database()").Scan(&name); err != nil {
		t.Fatal(err)
	}
	// The test's database is its own, and its name needs no quoting.
	if _, err := stores[0].db.ExecContext(t.Context(), "ALTER DATABASE "+name+" SET idle_session_timeout = '500ms'"); err != nil {
		t.Fatal(err)
	}
	l := listen(t, postgresBackend.connect(t, where))
	for setting, want := range map[string]string{"idle_session_timeout": "0", "application_name": holderName()} {
		var got string
		if err := l.conn.QueryRowContext(t.Context(), "SELECT current_setting($1)", setting).Scan(&got); err != nil || got != want {
			t.Errorf("the listener's %s = %q, %v, want %q", setting, got, err, want)
		}
	}
	time.Sleep(time.Second)
	// A new store, whose connection isn't idle yet.
	if _, err := postgresBackend.connect(t, where).Audit(t.Context(), started(time.Now(), "test")); err != nil {
		t.Fatal(err)
	}
	if err := wait(t, l, 5*time.Second); err != nil {
		t.Errorf("Wait() after a second idle = %v, want the change", err)
	}
}

// TestListenerLost checks that a listener whose session ends says so, as a
// failure rather than a deadline.
func TestListenerLost(t *testing.T) {
	stores, _ := replicas(t, 1)
	l := listen(t, stores[0])
	var pid int32
	if err := l.conn.QueryRowContext(t.Context(), "SELECT pg_backend_pid()").Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if _, err := stores[0].db.ExecContext(t.Context(), "SELECT pg_terminate_backend($1)", pid); err != nil {
		t.Fatal(err)
	}
	err := wait(t, l, 5*time.Second)
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Wait() on a listener whose session ended = %v, want a failure", err)
	}
	if err := l.Check(t.Context()); err == nil {
		t.Error("Check() on a listener whose session ended succeeded")
	}
}

// TestRelease checks that a leader that releases its lock lets another
// replica take it at once, before its session ends, and that the replicas
// that listen hear of it; and that it can't write as the leader after.
func TestRelease(t *testing.T) {
	stores, _ := replicas(t, 2)
	a, b := join(t, stores[0], "a"), join(t, stores[1], "b")
	if ta, err := a.TryLead(t.Context(), time.Time{}); err != nil || ta == nil {
		t.Fatalf("a's TryLead() = %+v, %v", ta, err)
	}
	l := listen(t, stores[1])
	if err := a.Release(t.Context()); err != nil {
		t.Fatalf("Release() = %v", err)
	}
	if err := wait(t, l, 5*time.Second); err != nil {
		t.Errorf("Wait() after the release = %v, want its notification", err)
	}
	if _, _, err := stores[0].RecordRefresh(t.Context(), full(time.Now())); !errors.Is(err, ErrNotLeader) {
		t.Errorf("a's RecordRefresh() after its release: error = %v, want ErrNotLeader", err)
	}
	if tb, err := b.TryLead(t.Context(), time.Time{}); err != nil || tb == nil || tb.Previous == nil || tb.Previous.ID != "A-ID" {
		t.Errorf("b's TryLead() after a's release = %+v, %v, want a takeover from a", tb, err)
	}
	if _, err := a.Heartbeat(t.Context(), time.Time{}); err == nil {
		t.Error("a's Heartbeat() after b took over succeeded")
	}
}
