package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store/sqlite"
)

// sessionSettings sets the settings of a replica's session, which bound how
// long PostgreSQL keeps its locks once the replica is lost (ADR-0047): the
// keepalives end a dead host's session within about 25 seconds, the user
// timeout one whose data the replica never acknowledges, and the
// idle-session timeout a hung process's, whose kernel still answers the
// keepalives, within 30. It names the session $1, as Lock's are named.
const sessionSettings = `SELECT set_config('application_name', $1, false),
  set_config('tcp_keepalives_idle', '10', false), set_config('tcp_keepalives_interval', '5', false),
  set_config('tcp_keepalives_count', '3', false), set_config('tcp_user_timeout', '25s', false),
  set_config('idle_session_timeout', '30s', false)`

// holdsLock reports whether the session whose backend PID is $1 holds the
// exclusive advisory lock on this database whose bigint key's high 32 bits
// are $2, and low ones $3.
const holdsLock = `SELECT EXISTS (SELECT 1 FROM pg_locks
WHERE locktype = 'advisory' AND granted AND mode = 'ExclusiveLock' AND objsubid = 1 AND pid = $1
  AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
  AND classid::bigint = $2 AND objid::bigint = $3)`

// ErrNotLeader is the error of a leader-only write by a replica that doesn't
// hold the leader's lock (ADR-0047).
var ErrNotLeader = errors.New("this replica doesn't hold the leader's lock, so it can't write as the leader")

// A Replica is one nbpdns serve among those on a PostgreSQL database
// (ADR-0047).
type Replica struct {
	// Name is its host's name, and ID a random one from its start.
	Name, ID string
}

// A Leader is the leader's row: the replica that leads, or last led.
type Leader struct {
	Replica
	// Since is when it took over, and LastSeen when it last confirmed that
	// it held the leader's lock.
	Since, LastSeen time.Time
	// NextRefresh is when its next refresh is due, or zero if none is
	// scheduled.
	NextRefresh time.Time
}

// A Takeover is what TryLead wrote when it took the leader's lock.
type Takeover struct {
	// Previous is the leader before, or nil if no replica had led the
	// database.
	Previous *Leader
	// Leader is the new leader's row.
	Leader Leader
	// Events are the takeover's audit events, once committed: its
	// service.leader_acquired.
	Events []audit.Event
}

// A Session is a replica's own session on PostgreSQL (ADR-0047), on a
// connection outside the store's pool, which holds the replica's locks: the
// serve lock, shared with the other replicas, and, on the leader, the
// leader's lock. Its settings bound how long PostgreSQL keeps them once the
// replica is lost.
type Session struct {
	s    *Store
	me   Replica
	conn *sql.Conn
	// pid is the session's backend's, as pg_locks names it.
	pid int32
	// key is the leader's lock's, which the database's ID gives, once
	// TryLead has read it.
	key   int64
	ended sync.Once
}

// Join opens a session of its own for me, a replica of serve on the store's
// PostgreSQL database, and takes the serve lock in it, shared with the other
// replicas. The commands that need no serve to run, such as db migrate, take
// it exclusively, so they refuse while a replica runs, and Join fails with a
// *LockedError while one of them does. It doesn't wait. Close ends the
// session.
//
// From the first Join, the store's leader-only writes check that one of its
// sessions holds the leader's lock.
func (s *Store) Join(ctx context.Context, me Replica) (*Session, error) {
	if s.dialect != postgresDialect {
		return nil, errors.New("joining the database's replicas: only PostgreSQL has them")
	}
	s.replica.Store(true)
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("joining the database's replicas: %w", err)
	}
	ss := &Session{s: s, me: me, conn: conn}
	if _, err := conn.ExecContext(ctx, sessionSettings, holderName()); err != nil {
		discard(conn)
		return nil, fmt.Errorf("joining the database's replicas: setting the session's timeouts: %w", err)
	}
	var locked bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock_shared($1), pg_backend_pid()", serveLockKey).Scan(&locked, &ss.pid); err != nil {
		discard(conn)
		return nil, fmt.Errorf("joining the database's replicas: %w", err)
	}
	if !locked {
		err := lockedError(ctx, conn)
		discard(conn)
		return nil, err
	}
	return ss, nil
}

// TryLead tries for the leader's lock on the session, without waiting, and
// returns nil if another replica holds it. If it takes it, it writes the
// leader's row, with nextRefresh, and a service.leader_acquired event, with
// the leader before, in a leader-only write, and returns the takeover. If
// that write fails, it releases the lock.
func (ss *Session) TryLead(ctx context.Context, nextRefresh time.Time) (*Takeover, error) {
	if ss.key == 0 {
		info, err := ss.s.Info(ctx)
		if err != nil {
			return nil, err
		}
		ss.key = leaderKey(info.ID)
	}
	var took bool
	if err := ss.conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", ss.key).Scan(&took); err != nil {
		return nil, fmt.Errorf("trying for the leader's lock: %w", err)
	}
	if !took {
		return nil, nil
	}
	ss.s.leading.Store(ss)
	t, err := ss.takeOver(ctx, nextRefresh)
	if err != nil {
		ss.s.leading.CompareAndSwap(ss, nil)
		// If the session has ended, so has its lock.
		_, _ = ss.conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", ss.key)
		return nil, err
	}
	return t, nil
}

// takeOver writes the leader's row, and the takeover's event.
func (ss *Session) takeOver(ctx context.Context, nextRefresh time.Time) (*Takeover, error) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	t := &Takeover{Leader: Leader{Replica: ss.me, Since: now, LastSeen: now, NextRefresh: nextRefresh}}
	events, err := ss.s.leaderTx(ctx, func(tx *Tx) error {
		var before json.RawMessage
		switch r, err := tx.q.GetLeader(ctx); {
		case err == nil:
			if t.Previous, err = leaderOf(r); err != nil {
				return err
			}
			p := t.Previous
			if before, err = audit.Value(map[string]any{"replica": p.Name, "instance_id": p.ID, "last_seen": p.LastSeen}); err != nil {
				return err
			}
		case !errors.Is(err, sql.ErrNoRows):
			return err
		}
		if err := tx.q.SetLeader(ctx, sqlite.SetLeaderParams{
			Replica: ss.me.Name, InstanceID: ss.me.ID, Since: now.Format(TimeLayout), LastSeen: now.Format(TimeLayout),
			NextRefresh: timeText(nextRefresh),
		}); err != nil {
			return err
		}
		after, err := audit.Value(map[string]string{"replica": ss.me.Name, "instance_id": ss.me.ID})
		if err != nil {
			return err
		}
		_, err = tx.Audit(ctx, audit.Event{
			Time: now, Actor: audit.System(), Action: audit.ServiceLeaderAcquired,
			Target: audit.Target{Type: "service", ID: ss.me.Name}, Before: before, After: after,
		}.WithContext(ctx))
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("taking over as the leader: %w", err)
	}
	t.Events = events
	return t, nil
}

// Heartbeat confirms, on the session, that its replica still leads: it
// updates the leader's row with when it was last seen, which it returns, and
// nextRefresh. It fails if the session has ended, and with it the lock, or
// if the row names another replica. Its caller then steps down.
func (ss *Session) Heartbeat(ctx context.Context, nextRefresh time.Time) (time.Time, error) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	n, err := ss.s.dialect.queries(ss.conn).LeaderHeartbeat(ctx, sqlite.LeaderHeartbeatParams{
		LastSeen: now.Format(TimeLayout), NextRefresh: timeText(nextRefresh), InstanceID: ss.me.ID,
	})
	switch {
	case err != nil:
		return time.Time{}, fmt.Errorf("the leader's heartbeat: %w", err)
	case n != 1:
		return time.Time{}, errors.New("the leader's heartbeat: the leader's row names another replica")
	}
	return now, nil
}

// Close ends the session, and with it the replica's locks: the leader's, if
// it holds it, which another replica can then take.
func (ss *Session) Close() {
	ss.s.leading.CompareAndSwap(ss, nil)
	ss.ended.Do(func() { discard(ss.conn) })
}

// Leader returns the leader's row, or nil if no replica has led the
// database.
func (s *Store) Leader(ctx context.Context) (*Leader, error) {
	r, err := s.q.GetLeader(ctx)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("reading the leader's row: %w", err)
	}
	l, err := leaderOf(r)
	if err != nil {
		return nil, fmt.Errorf("reading the leader's row: %w", err)
	}
	return l, nil
}

// leaderOf returns the leader that r stores.
func leaderOf(r sqlite.GetLeaderRow) (*Leader, error) {
	l := &Leader{Replica: Replica{Name: r.Replica, ID: r.InstanceID}}
	var err error
	if l.Since, err = time.Parse(TimeLayout, r.Since); err != nil {
		return nil, err
	}
	if l.LastSeen, err = time.Parse(TimeLayout, r.LastSeen); err != nil {
		return nil, err
	}
	if r.NextRefresh != "" {
		if l.NextRefresh, err = time.Parse(TimeLayout, r.NextRefresh); err != nil {
			return nil, err
		}
	}
	return l, nil
}

// leaderTx runs fn as InTx does, as a leader-only write (ADR-0047): once a
// replica has joined the database through the store, it first checks, in
// the transaction, after the write lock, that the store's session that
// leads still holds the leader's lock, and fails with ErrNotLeader if none
// does. A deposed leader that hasn't noticed yet, such as a process that
// resumes after a pause, can't write. On SQLite, where serve always leads
// under its flock, and on a store that no replica joined, such as the
// command line's, it checks nothing.
func (s *Store) leaderTx(ctx context.Context, fn func(tx *Tx) error) ([]audit.Event, error) {
	return s.InTx(ctx, func(tx *Tx) error {
		if s.replica.Load() {
			ss := s.leading.Load()
			if ss == nil {
				return ErrNotLeader
			}
			var held bool
			if err := tx.tx.QueryRowContext(ctx, holdsLock, ss.pid, ss.key>>32, ss.key&0xffff_ffff).Scan(&held); err != nil {
				return fmt.Errorf("checking the leader's lock: %w", err)
			}
			if !held {
				return ErrNotLeader
			}
		}
		return fn(tx)
	})
}

// leaderKey returns the key of the leader's lock on the database whose ID is
// id: 56 bits of a hash of it, so that two installs that share a PostgreSQL
// database, each in a schema of its own, never share the lock, as they
// would a constant.
func leaderKey(id string) int64 {
	sum := sha256.Sum256([]byte("nbpdns leader " + id))
	var k int64
	for _, b := range sum[:7] {
		k = k<<8 | int64(b)
	}
	return k
}

// timeText returns t as stored, or empty if it's zero.
func timeText(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(TimeLayout)
}
