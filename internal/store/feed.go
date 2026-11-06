package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/stdlib"
)

// channel is what every change notifies on PostgreSQL, and what a Listener
// listens on (ADR-0048). Notifications belong to their database, so two
// installs that share one, each in a schema of its own, hear each other's,
// which costs each a check.
const channel = "nbpdns"

// notify is the statement that a write transaction runs last on PostgreSQL,
// to tell the replicas of its change once it commits.
const notify = "NOTIFY " + channel

// Versions are what the change feed checks, for each replica to reload what
// moved (ADR-0048).
type Versions struct {
	// Settings and Groups are the settings' and the server groups'
	// versions, which each change bumps.
	Settings, Groups int64
	// Refreshes is the refreshes' version, the last refresh's ID, which
	// each refresh recorded moves.
	Refreshes int64
	// Webhooks is the webhooks' version, which each webhook that a replica
	// takes moves, as does each refresh that covers their queue.
	Webhooks int64
	// Leader is the leader's row, or nil if no replica has led the database,
	// as on SQLite.
	Leader *Leader
}

// Versions reads the versions that every replica follows, in one read
// transaction, so that each is as of one moment.
func (s *Store) Versions(ctx context.Context) (Versions, error) {
	sqlTx, q, err := s.readTx(ctx)
	if err != nil {
		return Versions{}, fmt.Errorf("checking the database for changes: %w", err)
	}
	defer func() { _ = sqlTx.Rollback() }()
	var v Versions
	if v.Settings, err = q.SequenceValue(ctx, seqSettings); err != nil {
		return Versions{}, fmt.Errorf("reading the settings' version: %w", err)
	}
	if v.Groups, err = q.SequenceValue(ctx, seqGroupsVersion); err != nil {
		return Versions{}, fmt.Errorf("reading the server groups' version: %w", err)
	}
	if v.Refreshes, err = q.SequenceValue(ctx, seqRefreshes); err != nil {
		return Versions{}, fmt.Errorf("reading the refreshes' version: %w", err)
	}
	if v.Webhooks, err = q.SequenceValue(ctx, seqQueue); err != nil {
		return Versions{}, fmt.Errorf("reading the webhooks' version: %w", err)
	}
	if v.Leader, err = readLeader(ctx, q); err != nil {
		return Versions{}, err
	}
	return v, nil
}

// A Listener hears of the changes to a PostgreSQL database, which every
// write transaction notifies, on a connection of its own (ADR-0048).
type Listener struct {
	conn *sql.Conn
}

// Listen opens a connection of its own to the store's PostgreSQL database,
// and listens on it for the changes that every replica notifies. Close ends
// it.
func (s *Store) Listen(ctx context.Context) (*Listener, error) {
	if s.dialect != postgresDialect {
		return nil, errors.New("listening for the database's changes: only PostgreSQL has LISTEN and NOTIFY")
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("listening for the database's changes: %w", err)
	}
	// The connection waits idle, so the role's or the database's
	// idle_session_timeout, if either sets one, mustn't end it. It's named
	// as the replica's session is, for pg_stat_activity.
	if _, err := conn.ExecContext(ctx, "SELECT set_config('application_name', $1, false), set_config('idle_session_timeout', '0', false)",
		holderName()); err != nil {
		discard(conn)
		return nil, fmt.Errorf("listening for the database's changes: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "LISTEN "+channel); err != nil {
		discard(conn)
		return nil, fmt.Errorf("listening for the database's changes: %w", err)
	}
	return &Listener{conn: conn}, nil
}

// Wait waits for a change to be notified, and returns nil once one is, or
// ctx's error if it's done first, after which the listener can wait again.
// Notifications that came in meanwhile, such as during Check, return at
// once, one each time. Any other error means the connection is lost: Close
// it, and listen again.
func (l *Listener) Wait(ctx context.Context) error {
	err := l.conn.Raw(func(dc any) error {
		pc, ok := dc.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("pgx's driver gave a %T", dc)
		}
		// pgx keeps the connection when ctx ends the wait: it only sets a
		// deadline on the socket.
		_, err := pc.Conn().WaitForNotification(ctx)
		return err
	})
	if err != nil && ctx.Err() == nil {
		return fmt.Errorf("waiting for the database's changes: %w", err)
	}
	return err
}

// Check checks that the listener's connection still works, for a connection
// that may have been lost without a word, such as to a network that fails.
func (l *Listener) Check(ctx context.Context) error {
	if err := l.conn.PingContext(ctx); err != nil {
		return fmt.Errorf("checking the connection that listens for the database's changes: %w", err)
	}
	return nil
}

// Close ends the listener's connection.
func (l *Listener) Close() { discard(l.conn) }

// Release releases the leader's lock, which the session holds, and notifies
// the change, so that a replica that listens takes it at once (ADR-0048):
// the session ending would release it too, but a notification sent then
// could reach the replicas before PostgreSQL has. It ends the store's
// leader-only writes on the session first. Close ends the session after.
func (ss *Session) Release(ctx context.Context) error {
	ss.s.leading.CompareAndSwap(ss, nil)
	// The notification goes once the statement commits, after the unlock.
	if _, err := ss.conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1), pg_notify($2, '')", ss.key, channel); err != nil {
		return fmt.Errorf("releasing the leader's lock: %w", err)
	}
	return nil
}
