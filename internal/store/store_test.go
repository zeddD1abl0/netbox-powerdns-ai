package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// A backend is a database that the store's tests run on: SQLite, and, with
// the build tag integration, the lab's PostgreSQL too
// (postgres_integration_test.go). Each test gets a new database.
type backend struct {
	name string
	// create makes a new, empty database, which it removes when the test
	// ends, and returns where it is.
	create func(t *testing.T) string
	// connect opens the database at where, as another process would, and
	// closes it when the test ends.
	connect func(t *testing.T, where string) *Store
}

// backends are the databases that each test runs on, in turn.
var backends = []backend{sqliteBackend}

// sqliteBackend makes each database in the test's directory.
var sqliteBackend = backend{
	name:   "sqlite",
	create: func(t *testing.T) string { return filepath.Join(t.TempDir(), "nbpdns.db") },
	connect: func(t *testing.T, where string) *Store {
		t.Helper()
		s, err := Open(t.Context(), where, Options{Create: true})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = s.Close() })
		return s
	},
}

// onEach runs test on each backend, as a subtest named after it.
func onEach(t *testing.T, test func(t *testing.T, db backend)) {
	t.Helper()
	for _, db := range backends {
		t.Run(db.name, func(t *testing.T) { test(t, db) })
	}
}

// open opens a new, empty database.
func (b backend) open(t *testing.T) *Store {
	t.Helper()
	return b.connect(t, b.create(t))
}

// migrated opens a new database with every migration applied.
func (b backend) migrated(t *testing.T) *Store {
	t.Helper()
	s := b.open(t)
	if _, err := s.Migrate(t.Context(), "test"); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestOpenCreatesTheDatabase(t *testing.T) {
	s := sqliteBackend.open(t)
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
	onEach(t, func(t *testing.T, db backend) {
		s := db.open(t)
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
	})
}

// TestMigrateFromEachVersion applies every migration to a database at each
// earlier version, as an upgrade from each release would.
func TestMigrateFromEachVersion(t *testing.T) {
	onEach(t, func(t *testing.T, db backend) {
		p, err := db.open(t).provider()
		if err != nil {
			t.Fatal(err)
		}
		for _, src := range p.ListSources() {
			t.Run(fmt.Sprint(src.Version), func(t *testing.T) {
				s := db.open(t)
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
	})
}

func TestMigrateRefusesANewerDatabase(t *testing.T) {
	onEach(t, func(t *testing.T, db backend) {
		s := db.open(t)
		ctx := t.Context()
		if _, err := s.Migrate(ctx, "test"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.ExecContext(ctx, "INSERT INTO goose_db_version (version_id, is_applied) VALUES (99999, TRUE)"); err != nil {
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
	})
}

// TestConcurrentWriters appends to the audit trail from two stores on one
// database at once, two writers on each, as the command line does while
// serve runs, and replicas do on PostgreSQL: each write waits for the
// others, none fails, and the trail they leave is one unbroken chain, which
// `nbpdns audit verify` checks.
func TestConcurrentWriters(t *testing.T) {
	onEach(t, func(t *testing.T, db backend) {
		ctx := t.Context()
		where := db.create(t)
		stores := []*Store{db.connect(t, where), db.connect(t, where)}
		if _, err := stores[0].Migrate(ctx, "test"); err != nil {
			t.Fatal(err)
		}
		const writers, each = 2, 25
		var wg sync.WaitGroup
		errs := make(chan error, len(stores)*writers*each)
		for i, s := range stores {
			for w := range writers {
				wg.Go(func() {
					for n := range each {
						_, err := s.Audit(ctx, started(time.Now(), fmt.Sprintf("host-%d-%d-%d", i, w, n)))
						errs <- err
					}
				})
			}
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		sum, err := stores[1].VerifyAudit(ctx)
		if err != nil {
			t.Fatalf("VerifyAudit() = %v", err)
		}
		if want := len(stores) * writers * each; sum.Count != want || sum.Last.Seq != int64(want) {
			t.Errorf("VerifyAudit() found %d events, the last %d; want %d", sum.Count, sum.Last.Seq, want)
		}
	})
}

// TestConcurrentMigrations migrates one database from several stores at
// once, as replicas that start together do: goose's lock lets one migrate at
// a time, and each migration is applied once.
func TestConcurrentMigrations(t *testing.T) {
	onEach(t, func(t *testing.T, db backend) {
		if db.name == sqliteBackend.name {
			t.Skip("on SQLite, one serve migrates, under its flock (ADR-0038)")
		}
		ctx := t.Context()
		where := db.create(t)
		const replicas = 4
		results := make([][]Migration, replicas)
		errs := make([]error, replicas)
		var wg sync.WaitGroup
		for i := range replicas {
			s := db.connect(t, where)
			wg.Go(func() { results[i], errs[i] = s.Migrate(ctx, "test") })
		}
		wg.Wait()
		applied := map[int64]int{}
		for i := range replicas {
			if errs[i] != nil {
				t.Fatalf("replica %d: Migrate() = %v", i, errs[i])
			}
			for _, m := range results[i] {
				applied[m.Version]++
			}
		}
		s := db.connect(t, where)
		st, err := s.Status(ctx)
		if err != nil || st.Pending() {
			t.Fatalf("Status() = %+v, %v, want nothing pending", st, err)
		}
		for _, m := range st.Migrations {
			if applied[m.Version] != 1 {
				t.Errorf("migration %d was applied %d times, want once", m.Version, applied[m.Version])
			}
		}
		var rows int
		if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM goose_db_version WHERE version_id > 0").Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if rows != len(st.Migrations) {
			t.Errorf("goose_db_version has %d rows, want %d", rows, len(st.Migrations))
		}
	})
}
