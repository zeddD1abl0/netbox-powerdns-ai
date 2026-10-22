package store

import (
	"strings"
	"testing"
	"time"
)

// TestJoinOnSQLite checks that serve on SQLite has no replicas to join, so
// that its leader-only writes check nothing.
func TestJoinOnSQLite(t *testing.T) {
	s := sqliteBackend.migrated(t)
	if _, err := s.Join(t.Context(), Replica{Name: "host-a", ID: "AAAA"}); err == nil || !strings.Contains(err.Error(), "only PostgreSQL has them") {
		t.Errorf("Join() on SQLite: error = %v", err)
	}
	if _, _, err := s.RecordRefresh(t.Context(), full(time.Now())); err != nil {
		t.Errorf("RecordRefresh() on SQLite: %v", err)
	}
}

func TestLeaderKey(t *testing.T) {
	a, b := leaderKey("KJ4G3RZQ6V7XHN2WYB5T5DM7QE"), leaderKey("M5NCAJ2PDB4VE6RXQW7HKTZ3GY")
	if a != leaderKey("KJ4G3RZQ6V7XHN2WYB5T5DM7QE") {
		t.Error("leaderKey() differs for one ID")
	}
	if a == b {
		t.Error("leaderKey() is the same for two IDs")
	}
	// A key with its top bits clear splits into classid and objid as
	// PostgreSQL shows them, unsigned.
	for _, k := range []int64{a, b} {
		if k <= 0 || k >= 1<<56 {
			t.Errorf("leaderKey() = %#x, want 56 bits", k)
		}
		if k == serveLockKey || k == writeLockKey {
			t.Errorf("leaderKey() = %#x, another lock's key", k)
		}
	}
}

func TestLockedErrorNamesEachHolder(t *testing.T) {
	for _, tt := range []struct {
		e    LockedError
		want string
	}{
		{LockedError{Holder: "host a, PID 1", Holders: 1},
			"another nbpdns holds the database's lock: host a, PID 1; stop it first, or give this one another database.url"},
		{LockedError{Holder: "host a, PID 1; host b, PID 2", Holders: 2},
			"2 other nbpdns processes hold the database's lock: host a, PID 1; host b, PID 2; stop them first, or give this one another database.url"},
		{LockedError{Path: "/var/lib/nbpdns/nbpdns.db.lock", Holder: "host a, PID 1"},
			"another nbpdns holds the database's lock, /var/lib/nbpdns/nbpdns.db.lock: host a, PID 1; stop it first"},
		{LockedError{}, "another nbpdns holds the database's lock: a process that didn't say which; stop it first"},
	} {
		if got := tt.e.Error(); !strings.HasPrefix(got, tt.want) {
			t.Errorf("%+v.Error() = %q, want it to start %q", tt.e, got, tt.want)
		}
	}
}
