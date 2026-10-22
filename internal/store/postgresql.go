package store

import (
	"context"
	"crypto/tls"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

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

// Supported lists the PostgreSQL major releases that nbpdns supports
// (REQ-054), newest first. The lab runs each one (internal/lab), and the
// integration tests run the store's tests on it, so a release is only added
// here with its lab instance.
var Supported = []string{"18"}

// SupportedRelease reports whether release, a PostgreSQL server's, such as
// 18.1, is of a supported major release.
func SupportedRelease(release string) bool {
	major, _, _ := strings.Cut(release, ".")
	return slices.Contains(Supported, major)
}

// SupportedSeries returns the supported releases as text, such as "18.x",
// or "18.x or 17.x" for more than one.
func SupportedSeries() string {
	s := make([]string, len(Supported))
	for i, r := range Supported {
		s[i] = r + ".x"
	}
	return strings.Join(s, " or ")
}

// postgresDialect is PostgreSQL's (ADR-0046).
var postgresDialect = &dialect{
	name:    PostgreSQL,
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
// libpq's parameters and defaults, the files they name, such as sslcert and
// passfile, and libpq's PG* environment variables, below url's own values.
// It connects once, to check the connection, and to learn whether it uses
// TLS, and the server's release. It applies no migrations; Migrate does.
//
// Its errors never hold url's password: pgx's show url with its password
// masked, or only its user and database.
func OpenPostgres(ctx context.Context, url string) (*Store, error) {
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("opening the database: %w", err)
	}
	db := stdlib.OpenDB(*cfg)
	c, release, err := connInfo(ctx, db)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("opening the database: %w", err)
	}
	s := newStore(db, postgresDialect, "")
	s.conn, s.release = c, release
	return s, nil
}

// connInfo connects to db, and returns how, and the server's release, such
// as 18.1.
func connInfo(ctx context.Context, db *sql.DB) (c connection, release string, err error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return c, "", err
	}
	defer conn.Close()
	err = conn.Raw(func(dc any) error {
		pc, ok := dc.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("pgx's driver gave a %T", dc)
		}
		nc := pc.Conn().PgConn().Conn()
		_, c.encrypted = nc.(*tls.Conn)
		c.socket = nc.RemoteAddr().Network() == "unix"
		return nil
	})
	if err != nil {
		return c, "", err
	}
	// Such as "18.1 (Debian 18.1-1.pgdg13+1)".
	if err := conn.QueryRowContext(ctx, "SHOW server_version").Scan(&release); err != nil {
		return c, "", fmt.Errorf("reading the server's release: %w", err)
	}
	release, _, _ = strings.Cut(release, " ")
	return c, release, nil
}

// discard ends conn's session, rather than return it to the pool, where it
// would keep the session's advisory locks and settings.
func discard(conn *sql.Conn) {
	_ = conn.Raw(func(any) error { return driver.ErrBadConn })
	_ = conn.Close()
}

// WriteReference writes the PostgreSQL section of the supported versions
// page, docs/reference/supported-versions.md, from Supported. It follows the
// PowerDNS section (powerdns.WriteReference).
func WriteReference(w io.Writer) error {
	var b strings.Builder
	p := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	p("\n## PostgreSQL\n\n")
	p("`nbpdns serve` keeps its state in SQLite, which is built in, or in one of\n")
	p("these releases of PostgreSQL, named by `database.url`. The development lab\n")
	p("runs each one, and the integration tests run the store's tests on every one.\n\n")
	p("| PostgreSQL |\n|---|\n")
	for _, r := range Supported {
		p("| %s.x |\n", r)
	}
	p("\nEach command that opens a PostgreSQL database checks the server's release.\n")
	p("If it isn't in the table, the command logs a warning and carries on, since\n")
	p("the database may still work.\n\n")
	p("nbpdns connects with pgx, which reads libpq's URL, parameters and\n")
	p("environment variables. It connects directly, or through a pooler in session\n")
	p("mode, such as PgBouncer's `pool_mode = session`: a pooler in transaction\n")
	p("mode can't hold its session's locks.\n")
	_, err := io.WriteString(w, b.String())
	return err
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
