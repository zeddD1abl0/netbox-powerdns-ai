//go:build integration

package cli

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/lab"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/service"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
)

// TestFeedListens checks that the change feed's listener has the feed check
// once it listens, and at each change; and that it listens again once its
// connection is lost, after a poll interval, and has the feed check then
// (ADR-0048).
func TestFeedListens(t *testing.T) {
	where := lab.NewPostgresDatabase(t)
	st, err := store.OpenPostgres(t.Context(), where)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.Migrate(t.Context(), "test"); err != nil {
		t.Fatal(err)
	}
	s := testSession(t)
	logs := &lockedBuffer{}
	s.log = slog.New(slog.NewJSONHandler(logs, nil))
	f := &feed{s: s, st: st, interval: 200 * time.Millisecond, wake: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		f.listen(ctx)
	}()
	defer func() {
		cancel()
		<-done
	}()
	woken := func(when string) {
		t.Helper()
		select {
		case <-f.wake:
		case <-time.After(5 * time.Second):
			t.Fatalf("the feed wasn't woken %s:\n%s", when, logs)
		}
	}
	change := func() {
		t.Helper()
		if _, err := st.Audit(t.Context(), audit.Event{Actor: audit.System(), Action: audit.ServiceStarted,
			Target: audit.Target{Type: "service", ID: "test"}}); err != nil {
			t.Fatal(err)
		}
	}
	woken("once it listened")
	change()
	woken("by a change")

	// The listener's session ends, as when PostgreSQL restarts.
	db, err := sql.Open("pgx", where)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var pid int32
	if err := db.QueryRowContext(t.Context(), `SELECT pid FROM pg_stat_activity
WHERE datname = current_database() AND query = 'LISTEN nbpdns'`).Scan(&pid); err != nil {
		t.Fatalf("finding the listener's session: %v", err)
	}
	var ended bool
	if err := db.QueryRowContext(t.Context(), "SELECT pg_terminate_backend($1)", pid).Scan(&ended); err != nil || !ended {
		t.Fatalf("ending the listener's session: %v, %v", ended, err)
	}
	woken("once it listened again")
	change()
	woken("by a change after it listened again")
	for _, want := range []string{"listening for the database's changes", "lost the connection that listens for the database's changes",
		"listening for the database's changes again"} {
		if !strings.Contains(logs.String(), `"msg":"`+want) {
			t.Errorf("the log doesn't say %q:\n%s", want, logs)
		}
	}
}

// TestServeListens runs two replicas of serve on PostgreSQL with
// database.listen, and a poll interval longer than the test waits, and
// checks that a setting changed with the command line reaches both at once,
// and that the follower takes over from a leader that stops at once
// (ADR-0048).
func TestServeListens(t *testing.T) {
	env := configFile(t, twoGroups)
	env["NBPDNS_NETBOX_URL"], env["NBPDNS_NETBOX_TOKEN"] = "http://127.0.0.1:1", "nbt_abc.def"
	env["NBPDNS_DATABASE_URL"] = lab.NewPostgresDatabase(t)
	env["NBPDNS_DATABASE_POLL_INTERVAL"], env["NBPDNS_DATABASE_LISTEN"] = "30s", "true"
	first := startServe(t, env)
	first.statusWhen(t, func(st service.Status) bool { return st.Replica.Role == service.RoleLeader })
	second := startServe(t, env)
	follower := second.statusWhen(t, func(st service.Status) bool { return st.Leader != nil })
	for _, s := range []*served{first, second} {
		waitFor(t, s, "listening", func() bool {
			return strings.Contains(s.stderr.String(), `"msg":"listening for the database's changes"`)
		})
	}
	// Each of these happens at once, so within far less than the poll.
	const atOnce = 5 * time.Second

	changed := time.Now()
	other := with(env, "NBPDNS_SERVER_LISTEN", "127.0.0.1:0")
	if code, _, stderr := run(t, other, "settings", "set", "drift.interval", "2m", "--reason", "a test"); code != exitOK {
		t.Fatalf("settings set: exit %d:\n%s", code, stderr)
	}
	for _, s := range []*served{first, second} {
		s.statusWhen(t, func(st service.Status) bool { return st.Schedule.IntervalSeconds == 120 })
	}
	if d := time.Since(changed); d > atOnce {
		t.Errorf("the replicas applied the setting %s after it changed, want within %s", d, atOnce)
	}

	stopped := time.Now()
	if code := first.stop(); code != exitOK {
		t.Fatalf("exit %d:\n%s", code, first.stderr)
	}
	took := second.statusWhen(t, func(st service.Status) bool { return st.Replica.Role == service.RoleLeader })
	if d := time.Since(stopped); d > atOnce {
		t.Errorf("the follower took over %s after the leader stopped, want within %s", d, atOnce)
	}
	if took.Leader == nil || took.Leader.ID != follower.Replica.ID {
		t.Errorf("after the takeover: leader %+v, want the second serve", took.Leader)
	}
	if !strings.Contains(first.stderr.String(), "released the leader's lock, for another replica to take") {
		t.Errorf("the leader didn't release its lock as it stopped:\n%s", first.stderr)
	}
}
