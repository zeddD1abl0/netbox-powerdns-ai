package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store/postgres"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store/sqlite"
)

// writeLockKey is the key of the advisory lock that each write transaction
// takes on PostgreSQL: "nbpdns" in ASCII, then 1. Advisory locks belong to
// their database, so two databases on one server never share it.
const writeLockKey int64 = 0x6e62_7064_6e73_0001

// migrateLockPeriod is how often, in seconds, a migration that another one
// holds up tries for goose's lock again, and migrateLockTries how many times
// it tries, five minutes in all.
const (
	migrateLockPeriod = 1
	migrateLockTries  = 300
)

// postgresDialect is PostgreSQL's (ADR-0046).
var postgresDialect = &dialect{
	queries: func(db sqlite.DBTX) sqlite.Querier { return pgQueries{postgres.New(db)} },
	goose:   goose.DialectPostgres,
	// goose's session locker holds an advisory lock while it migrates, so
	// that replicas that start together migrate once. Its retries count up,
	// so each provider gets its own.
	gooseOptions: func() ([]goose.ProviderOption, error) {
		l, err := lock.NewPostgresSessionLocker(lock.WithLockTimeout(migrateLockPeriod, migrateLockTries))
		if err != nil {
			return nil, err
		}
		return []goose.ProviderOption{goose.WithSessionLocker(l)}, nil
	},
	// Read committed, whatever the database's default, so that each
	// statement after the lock sees the writes that committed before it.
	write:      &sql.TxOptions{Isolation: sql.LevelReadCommitted},
	lockWrites: lockWrites,
}

// OpenPostgres opens the PostgreSQL database that url names, in libpq's
// form: a postgres:// or postgresql:// URL, or keywords and values. pgx reads
// libpq's parameters, and its PG* environment variables. It applies no
// migrations; Migrate does.
func OpenPostgres(ctx context.Context, url string) (*Store, error) {
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		// pgx's error shows the URL, without its password.
		return nil, fmt.Errorf("opening the database: %w", err)
	}
	db := stdlib.OpenDB(*cfg)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("opening the database: %w", err)
	}
	return newStore(db, postgresDialect, ""), nil
}

// lockWrites takes the write lock, a transaction-scoped advisory lock, which
// the transaction's end releases, so that writes run one at a time, as
// SQLite's immediate transactions do (ADR-0046): the audit trail reads its
// last event, then appends the next one, and a sequence's users read it,
// then write. It waits for busyTimeout, as SQLite does.
func lockWrites(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, "SET LOCAL lock_timeout = "+strconv.FormatInt(busyTimeout.Milliseconds(), 10)); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", writeLockKey)
	return err
}
