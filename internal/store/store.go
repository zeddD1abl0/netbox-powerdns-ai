// Package store keeps the state of `nbpdns serve` in a database (ADR-0038):
// embedded SQLite, through the pure-Go modernc.org/sqlite driver, in WAL
// mode, so that the command line can write while serve runs. Its schema
// comes from goose's forward-only migrations, embedded in the binary, and
// its queries from sqlc, which generates ./sqlite from the .sql files here.
// The SQL is portable, so that M09 can run it on PostgreSQL.
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite" // the database/sql driver "sqlite"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store/sqlite"
)

// busyTimeout is how long a write waits for another one to finish, such as
// the command line's while serve writes a refresh.
const busyTimeout = 5 * time.Second

// TimeLayout is how times are stored: in UTC, to the microsecond, at a fixed
// width, so that they sort as text, on SQLite and PostgreSQL alike. It's the
// audit trail's, whose hashes cover times as stored.
const TimeLayout = audit.TimeLayout

// A Store is an open database.
type Store struct {
	db   *sql.DB
	q    *sqlite.Queries
	path string
}

// Options say how to open a database.
type Options struct {
	// Create makes the database if it doesn't exist. Otherwise, Open fails.
	Create bool
}

// Open opens the SQLite database at path. It applies no migrations; Migrate
// does.
func Open(ctx context.Context, path string, opts Options) (*Store, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("the database's path, %s: %w", path, err)
	}
	if err := prepare(abs, opts.Create); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn(abs))
	if err != nil {
		return nil, fmt.Errorf("opening the database, %s: %w", abs, err)
	}
	// Reading the schema checks that the file is a database.
	if _, err := db.ExecContext(ctx, "SELECT count(*) FROM sqlite_schema"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("opening the database, %s: %w", abs, err)
	}
	return &Store{db: db, q: sqlite.New(db), path: abs}, nil
}

// Path returns the database's file, as an absolute path.
func (s *Store) Path() string { return s.path }

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// checkDir checks that the directory of the database at path exists.
func checkDir(path string) error {
	dir := filepath.Dir(path)
	switch fi, err := os.Stat(dir); {
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("the database's directory, %s, doesn't exist; create it, or mount a volume there, writable by user %d",
			dir, os.Getuid())
	case err != nil:
		return fmt.Errorf("the database's directory: %w", err)
	case !fi.IsDir():
		return fmt.Errorf("the database's directory, %s, isn't a directory", dir)
	}
	return nil
}

// prepare checks the database's directory, and, if create is set, creates
// the database's file if it doesn't exist, readable and writable only by its
// owner. SQLite gives its WAL files the same permissions.
func prepare(path string, create bool) error {
	if err := checkDir(path); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	switch _, err := os.Stat(path); {
	case err == nil:
		return nil
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("the database: %w", err)
	case !create:
		return fmt.Errorf("there's no database at %s; nbpdns serve, or nbpdns db migrate, creates it", path)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	switch {
	case errors.Is(err, fs.ErrExist):
		return nil // made since the check
	case errors.Is(err, fs.ErrPermission):
		return fmt.Errorf("creating the database: %w; make %s writable by user %d", err, dir, os.Getuid())
	case err != nil:
		return fmt.Errorf("creating the database: %w", err)
	}
	return f.Close()
}

// dsn returns the driver's name for the database at path, an absolute path:
// a file: URI, which SQLite reads, with the pragmas each connection starts
// with, and immediate transactions, which take the write lock when they
// begin, so that two writers wait for each other rather than fail.
// Synchronous FULL makes each committed transaction, such as an audit
// event, survive a power cut.
func dsn(path string) string {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout("+strconv.FormatInt(busyTimeout.Milliseconds(), 10)+")")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(FULL)")
	q.Add("_pragma", "foreign_keys(ON)")
	q.Set("_txlock", "immediate")
	return (&url.URL{Scheme: "file", Path: path}).String() + "?" + q.Encode()
}

// Info is the database's identity, written when it's created.
type Info struct {
	// ID is random, and tells one database from another.
	ID string
	// Created is when nbpdns created the database.
	Created time.Time
	// CreatedBy is the version of nbpdns that created it.
	CreatedBy string
}

// Info returns the database's identity. It's set by the first Migrate.
func (s *Store) Info(ctx context.Context) (Info, error) {
	r, err := s.q.GetDatabaseInfo(ctx)
	if err != nil {
		return Info{}, fmt.Errorf("reading the database's identity: %w", err)
	}
	created, err := time.Parse(TimeLayout, r.CreatedAt)
	if err != nil {
		return Info{}, fmt.Errorf("reading the database's identity: %w", err)
	}
	return Info{ID: r.ID, Created: created, CreatedBy: r.CreatedBy}, nil
}

// initInfo writes the database's identity, unless it has one.
func (s *Store) initInfo(ctx context.Context, version string) error {
	err := s.q.InitDatabaseInfo(ctx, sqlite.InitDatabaseInfoParams{
		ID:        rand.Text(),
		CreatedAt: time.Now().UTC().Format(TimeLayout),
		CreatedBy: version,
	})
	if err != nil {
		return fmt.Errorf("writing the database's identity: %w", err)
	}
	return nil
}
