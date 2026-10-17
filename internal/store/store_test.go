package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// open opens a new database in a test's directory, and closes it when the
// test ends.
func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "nbpdns.db"), Options{Create: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestOpenCreatesTheDatabase(t *testing.T) {
	s := open(t)
	fi, err := os.Stat(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("the database's permissions = %v, want -rw-------", fi.Mode().Perm())
	}
	if !filepath.IsAbs(s.Path()) {
		t.Errorf("Path() = %q, want an absolute path", s.Path())
	}
	// Each connection starts with the pragmas.
	for pragma, want := range map[string]string{
		"journal_mode": "wal",
		"busy_timeout": "5000",
		"synchronous":  "2", // FULL
		"foreign_keys": "1",
	} {
		var got string
		if err := s.db.QueryRowContext(t.Context(), "PRAGMA "+pragma).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("PRAGMA %s = %s, want %s", pragma, got, want)
		}
	}
}

func TestOpenRelativePath(t *testing.T) {
	t.Chdir(t.TempDir())
	s, err := Open(t.Context(), "nbpdns.db", Options{Create: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	wd, _ := os.Getwd()
	if want := filepath.Join(wd, "nbpdns.db"); s.Path() != want {
		t.Errorf("Path() = %q, want %q", s.Path(), want)
	}
}

func TestOpenErrors(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	garbage := filepath.Join(dir, "garbage.db")
	for _, f := range []string{file, garbage} {
		if err := os.WriteFile(f, []byte(strings.Repeat("not a database ", 100)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name   string
		path   string
		create bool
		want   string
	}{
		{"no directory", filepath.Join(dir, "missing", "nbpdns.db"), true,
			"the database's directory, " + filepath.Join(dir, "missing") + ", doesn't exist; create it, or mount a volume there"},
		{"a file for a directory", filepath.Join(file, "nbpdns.db"), true, "isn't a directory"},
		{"no database, without create", filepath.Join(dir, "nbpdns.db"), false,
			"there's no database at " + filepath.Join(dir, "nbpdns.db") + "; nbpdns serve, or nbpdns db migrate, creates it"},
		{"not a database", garbage, true, "opening the database, " + garbage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Open(t.Context(), tt.path, Options{Create: tt.create})
			if err == nil {
				_ = s.Close()
				t.Fatal("Open() succeeded, want an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Open() error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(dir, "nbpdns.db")); err == nil {
		t.Error("Open() without create made the database")
	}
}

func TestOpenUnwritableDirectory(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can write to any directory")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil { //nolint:gosec // A directory nbpdns can't write to.
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) //nolint:gosec // The test's own directory, as it was.
	_, err := Open(t.Context(), filepath.Join(dir, "nbpdns.db"), Options{Create: true})
	want := fmt.Sprintf("make %s writable by user %d", dir, os.Getuid())
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("Open() error = %v, want it to contain %q", err, want)
	}
}

func TestMigrate(t *testing.T) {
	s := open(t)
	ctx := t.Context()
	before, err := s.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if before.Version != 0 || before.Latest < 1 || !before.Pending() {
		t.Fatalf("a new database's Status() = %+v, want version 0 and pending migrations", before)
	}
	applied, err := s.Migrate(ctx, "v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != len(before.Migrations) || applied[0].Version != 1 || applied[0].Name != "database" {
		t.Errorf("Migrate() applied %+v, want every migration, from 1, database", applied)
	}
	after, err := s.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Pending() || after.Version != after.Latest {
		t.Errorf("after Migrate(), Status() = %+v, want nothing pending", after)
	}
	for _, m := range after.Migrations {
		if m.Applied.IsZero() || time.Since(m.Applied) > time.Minute {
			t.Errorf("migration %d applied at %v, want just now", m.Version, m.Applied)
		}
	}
	info, err := s.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.ID) < 20 || info.CreatedBy != "v1.2.3" || time.Since(info.Created) > time.Minute {
		t.Errorf("Info() = %+v, want a random ID, v1.2.3, and now", info)
	}

	// Again, nothing to apply, and the identity stays.
	again, err := s.Migrate(ctx, "v9.9.9")
	if err != nil || len(again) != 0 {
		t.Errorf("Migrate() again = %+v, %v, want nothing applied", again, err)
	}
	if info2, err := s.Info(ctx); err != nil || info2 != info {
		t.Errorf("Info() after a second Migrate() = %+v, %v, want %+v", info2, err, info)
	}
}

// TestMigrateFromEachVersion applies every migration to a database at each
// earlier version, as an upgrade from each release would.
func TestMigrateFromEachVersion(t *testing.T) {
	p, err := open(t).provider()
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range p.ListSources() {
		t.Run(fmt.Sprint(src.Version), func(t *testing.T) {
			s := open(t)
			p, err := s.provider()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := p.UpTo(t.Context(), src.Version); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Migrate(t.Context(), "test"); err != nil {
				t.Fatalf("Migrate() from version %d: %v", src.Version, err)
			}
			st, err := s.Status(t.Context())
			if err != nil || st.Pending() {
				t.Errorf("Status() = %+v, %v, want nothing pending", st, err)
			}
		})
	}
}

func TestMigrateRefusesANewerDatabase(t *testing.T) {
	s := open(t)
	ctx := t.Context()
	if _, err := s.Migrate(ctx, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, "INSERT INTO goose_db_version (version_id, is_applied) VALUES (99999, 1)"); err != nil {
		t.Fatal(err)
	}
	_, err := s.Migrate(ctx, "test")
	var tn *TooNewError
	if !errors.As(err, &tn) || tn.Version != 99999 {
		t.Fatalf("Migrate() error = %v, want a TooNewError at 99999", err)
	}
	if !strings.Contains(err.Error(), "newer than this nbpdns knows") {
		t.Errorf("Migrate() error = %v", err)
	}
}

// TestConcurrentWriters writes from two stores on one database at once, as
// the command line does while serve runs: each write waits for the other's,
// and none fails as busy.
func TestConcurrentWriters(t *testing.T) {
	a := open(t)
	ctx := t.Context()
	if _, err := a.db.ExecContext(ctx, "CREATE TABLE counter (n BIGINT NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.db.ExecContext(ctx, "INSERT INTO counter (n) VALUES (0)"); err != nil {
		t.Fatal(err)
	}
	b, err := Open(ctx, a.Path(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	const each = 50
	var wg sync.WaitGroup
	errs := make(chan error, 2*each)
	for _, s := range []*Store{a, b} {
		wg.Go(func() {
			for range each {
				errs <- increment(ctx, s)
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := a.db.QueryRowContext(ctx, "SELECT n FROM counter").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2*each {
		t.Errorf("counter = %d, want %d", n, 2*each)
	}
}

// increment reads and writes the counter in one transaction.
func increment(ctx context.Context, s *Store) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // after Commit, it does nothing
	var n int
	if err := tx.QueryRowContext(ctx, "SELECT n FROM counter").Scan(&n); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE counter SET n = ?", n+1); err != nil {
		return err
	}
	return tx.Commit()
}
