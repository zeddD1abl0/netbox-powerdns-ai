package service

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
)

// jsonStatus returns s's status, as /status?json=1 serves it.
func jsonStatus(t *testing.T, s *Service) Status {
	t.Helper()
	code, body := get(t, s, http.MethodGet, "/status?json=1")
	var st Status
	if err := json.Unmarshal([]byte(body), &st); err != nil || code != http.StatusOK {
		t.Fatalf("status JSON: %d, %v:\n%s", code, err, body)
	}
	return st
}

// TestReplicaOnSQLite checks that a service with no election leads, as
// serve does on SQLite, since it started.
func TestReplicaOnSQLite(t *testing.T) {
	me := store.Replica{Name: "host-a", ID: "AAAA"}
	s, _, _ := testService(t, Options{Replica: me})
	next := time.Now().Add(time.Minute).UTC().Truncate(time.Second)
	s.mu.Lock()
	s.st.next = next
	s.mu.Unlock()

	st := jsonStatus(t, s)
	if want := (Replica{Name: "host-a", ID: "AAAA", Role: RoleLeader}); st.Replica != want {
		t.Errorf("replica = %+v, want %+v", st.Replica, want)
	}
	l := st.Leader
	if l == nil || l.Name != "host-a" || l.ID != "AAAA" || !l.Since.Equal(st.Started) || l.LastSeen.Before(st.Started) ||
		l.NextRefresh == nil || !l.NextRefresh.Equal(next) {
		t.Errorf("leader = %+v, want this replica, since it started, with its next refresh, %s", l, next)
	}
	if _, page := get(t, s, http.MethodGet, "/status"); !strings.Contains(page, "Replica: host-a, ID AAAA, the leader since ") {
		t.Errorf("the page doesn't name the replica as the leader:\n%s", page)
	}
}

// TestElected checks a replica's status and liveness as its election
// reports to it: a follower is live while it polls, and the leader while it
// refreshes.
func TestElected(t *testing.T) {
	clock := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	now := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return clock
	}
	advance := func(d time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		clock = clock.Add(d)
	}
	me := store.Replica{Name: "host-a", ID: "AAAA"}
	s, _, _ := testService(t, Options{now: now, Replica: me, Elected: true, Interval: 5 * time.Minute, Timeout: 10 * time.Minute})
	livez := func() (int, string) { return get(t, s, http.MethodGet, "/livez") }

	// Before the election reports, the replica follows no one, and is live
	// for a minute.
	st := jsonStatus(t, s)
	if st.Replica.Role != RoleFollower || st.Leader != nil || !st.Live {
		t.Errorf("before the election: replica %+v, leader %+v, live %t", st.Replica, st.Leader, st.Live)
	}
	if _, page := get(t, s, http.MethodGet, "/status"); !strings.Contains(page, "Replica: host-a, ID AAAA, a follower; no replica leads yet") {
		t.Errorf("the page before the election:\n%s", page)
	}
	advance(61 * time.Second)
	if code, body := livez(); code != http.StatusServiceUnavailable || !strings.Contains(body, "this follower hasn't tried for the leader's lock since 2026-10-22T12:00:00Z") {
		t.Errorf("livez a minute after the start, without a poll: %d %q", code, body)
	}

	// A follower is live while it polls, however long ago a refresh ran.
	other := store.Leader{Replica: store.Replica{Name: "host-b", ID: "BBBB"}, Since: clock.Add(-time.Hour), LastSeen: clock.Add(-2 * time.Second),
		NextRefresh: clock.Add(time.Minute)}
	s.Follow(&other)
	if code, _ := livez(); code != http.StatusOK {
		t.Errorf("livez after a poll: %d", code)
	}
	st = jsonStatus(t, s)
	want := Leader{Name: "host-b", ID: "BBBB", Since: other.Since, LastSeen: other.LastSeen, NextRefresh: &other.NextRefresh}
	if st.Replica.Role != RoleFollower || st.Leader == nil || st.Leader.Name != want.Name || st.Leader.ID != want.ID ||
		!st.Leader.Since.Equal(want.Since) || !st.Leader.LastSeen.Equal(want.LastSeen) || !st.Leader.NextRefresh.Equal(*want.NextRefresh) {
		t.Errorf("a follower: replica %+v, leader %+v, want %+v", st.Replica, st.Leader, want)
	}
	// The next refresh is the leader's, as its row has it (ADR-0048).
	if st.Schedule.NextRefresh == nil || !st.Schedule.NextRefresh.Equal(other.NextRefresh) {
		t.Errorf("a follower's next refresh = %v, want the leader's, %v", st.Schedule.NextRefresh, other.NextRefresh)
	}
	if _, page := get(t, s, http.MethodGet, "/status"); !strings.Contains(page,
		"Replica: host-a, ID AAAA, a follower of host-b, ID BBBB, the leader since 2026-10-22T11:01:01Z, last seen 2026-10-22T12:00:59Z") {
		t.Errorf("the page of a follower:\n%s", page)
	}

	// The leader is live while it refreshes, from when it took over.
	advance(time.Hour)
	s.Lead(store.Leader{Replica: me, Since: clock, LastSeen: clock})
	st = jsonStatus(t, s)
	if st.Replica.Role != RoleLeader || st.Leader == nil || st.Leader.ID != "AAAA" || st.Leader.NextRefresh != nil || !st.Live {
		t.Errorf("the leader: replica %+v, leader %+v, live %t", st.Replica, st.Leader, st.Live)
	}
	advance(16 * time.Minute)
	if code, _ := livez(); code != http.StatusOK {
		t.Errorf("livez 16 minutes after the takeover: %d", code)
	}
	advance(time.Minute)
	if code, body := livez(); code != http.StatusServiceUnavailable || !strings.Contains(body, "no drift refresh has started since") {
		t.Errorf("livez 17 minutes after the takeover, without a refresh: %d %q", code, body)
	}

	// A replica that steps down schedules no refresh.
	s.mu.Lock()
	s.st.next = clock.Add(time.Minute)
	s.mu.Unlock()
	if got := s.NextRefresh(); !got.Equal(clock.Add(time.Minute)) {
		t.Errorf("NextRefresh() on the leader = %v", got)
	}
	s.Follow(nil)
	if got := s.NextRefresh(); !got.IsZero() {
		t.Errorf("NextRefresh() after stepping down = %v, want zero", got)
	}
	if st := jsonStatus(t, s); st.Replica.Role != RoleFollower || st.Leader != nil || !st.Live || st.Schedule.NextRefresh != nil {
		t.Errorf("after stepping down: replica %+v, leader %+v, live %t, next refresh %v", st.Replica, st.Leader, st.Live, st.Schedule.NextRefresh)
	}
}

// TestNextRefresh checks that no refresh is due while one runs.
func TestNextRefresh(t *testing.T) {
	s, _, _ := testService(t, Options{})
	t0 := time.Now()
	s.mu.Lock()
	s.st.next, s.st.lastStart, s.st.lastEnd = t0.Add(time.Minute), t0.Add(-time.Minute), t0
	s.mu.Unlock()
	if got := s.NextRefresh(); !got.Equal(t0.Add(time.Minute)) {
		t.Errorf("NextRefresh() between refreshes = %v, want %v", got, t0.Add(time.Minute))
	}
	s.mu.Lock()
	s.st.lastStart = t0.Add(time.Second)
	s.mu.Unlock()
	if got := s.NextRefresh(); !got.IsZero() {
		t.Errorf("NextRefresh() during a refresh = %v, want zero", got)
	}
}
