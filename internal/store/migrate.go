package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
)

// migrations are the schema's migrations, in goose's SQL format, applied in
// the order of their numbers. They only move forward (ADR-0038): each has an
// Up section, and none has a Down. Never change one that a release has
// shipped; add another.
//
//go:embed migrations/*.sql
var migrations embed.FS

// A Migration is one of the schema's migrations.
type Migration struct {
	// Version is its number.
	Version int64
	// Name is its file's name, without its number and extension, such as
	// database.
	Name string
	// Applied is when it was applied to the database, or zero if it's
	// pending.
	Applied time.Time
	// Duration is how long it took to apply, when Migrate applied it.
	Duration time.Duration
}

// Status is the database's schema, against the migrations that this nbpdns
// knows.
type Status struct {
	// Version is the database's: the last migration applied to it.
	Version int64
	// Latest is the last migration that this nbpdns knows.
	Latest int64
	// Migrations are every migration this nbpdns knows, in order.
	Migrations []Migration
}

// Pending reports whether the database lacks a migration.
func (s Status) Pending() bool { return s.Version < s.Latest }

// TooNewError is the error for a database that a newer nbpdns has migrated.
// Migrations only move forward, so this nbpdns mustn't use it.
type TooNewError struct{ Version, Latest int64 }

func (e *TooNewError) Error() string {
	return fmt.Sprintf("the database's schema is at version %d, newer than this nbpdns knows, %d; "+
		"run a newer nbpdns, or restore the backup taken before the upgrade", e.Version, e.Latest)
}

// provider returns goose's provider for the database.
func (s *Store) provider() (*goose.Provider, error) {
	fsys, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return nil, err
	}
	opts, err := s.dialect.gooseOptions()
	if err != nil {
		return nil, err
	}
	p, err := goose.NewProvider(s.dialect.goose, s.db, fsys, append(opts, goose.WithDisableGlobalRegistry(true))...)
	if err != nil {
		return nil, fmt.Errorf("reading the migrations: %w", err)
	}
	return p, nil
}

// Status returns the database's schema, against the migrations that this
// nbpdns knows.
func (s *Store) Status(ctx context.Context) (Status, error) {
	p, err := s.provider()
	if err != nil {
		return Status{}, err
	}
	sts, err := p.Status(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("reading the database's migrations: %w", err)
	}
	version, err := p.GetDBVersion(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("reading the database's schema version: %w", err)
	}
	st := Status{Version: version}
	for _, m := range sts {
		mig := Migration{Version: m.Source.Version, Name: migrationName(m.Source.Path)}
		if m.State == goose.StateApplied {
			mig.Applied = m.AppliedAt.UTC()
		}
		st.Migrations = append(st.Migrations, mig)
		st.Latest = max(st.Latest, m.Source.Version)
	}
	return st, nil
}

// Migrate applies the migrations that the database lacks, in order, each in
// its own transaction, and returns them. On PostgreSQL, it holds goose's
// advisory lock while it migrates, so that replicas that migrate at once
// apply each migration once. It fails, changing nothing, on a database
// whose schema is newer than this nbpdns knows. version, the
// running nbpdns's, is recorded if Migrate creates the database's identity.
func (s *Store) Migrate(ctx context.Context, version string) ([]Migration, error) {
	st, err := s.Status(ctx)
	if err != nil {
		return nil, err
	}
	if st.Version > st.Latest {
		return nil, &TooNewError{Version: st.Version, Latest: st.Latest}
	}
	p, err := s.provider()
	if err != nil {
		return nil, err
	}
	results, err := p.Up(ctx)
	if err != nil {
		// Each migration is a transaction, so those before the one that
		// failed stay applied.
		var pe *goose.PartialError
		if errors.As(err, &pe) {
			return applied(pe.Applied), fmt.Errorf("migrating the database: %s: %w", migrationName(pe.Failed.Source.Path), pe.Err)
		}
		return nil, fmt.Errorf("migrating the database: %w", err)
	}
	if err := s.initInfo(ctx, version); err != nil {
		return applied(results), err
	}
	return applied(results), nil
}

// applied returns the migrations that goose applied.
func applied(results []*goose.MigrationResult) []Migration {
	var ms []Migration
	for _, r := range results {
		ms = append(ms, Migration{Version: r.Source.Version, Name: migrationName(r.Source.Path), Duration: r.Duration})
	}
	return ms
}

// migrationName returns a migration's name from its file's: 00001_database.sql
// is database.
func migrationName(file string) string {
	name := strings.TrimSuffix(path.Base(file), path.Ext(file))
	if _, rest, ok := strings.Cut(name, "_"); ok {
		return rest
	}
	return name
}
