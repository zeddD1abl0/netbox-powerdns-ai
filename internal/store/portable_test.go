package store

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// sqliteOnly matches SQL that SQLite takes and PostgreSQL doesn't, or takes
// differently (ADR-0038). sqlc's PostgreSQL engine, which `make
// generate-check` runs on the same files, refuses SQLite's syntax, but not
// its functions, which it doesn't check.
var sqliteOnly = []struct {
	re   *regexp.Regexp
	note string
}{
	{regexp.MustCompile(`(?i)\bAUTOINCREMENT\b`), "nbpdns assigns its own IDs"},
	{regexp.MustCompile(`(?i)\bINTEGER\s+PRIMARY\s+KEY\b`), "SQLite makes it the rowid; use BIGINT, and assign the ID"},
	{regexp.MustCompile(`(?i)\bWITHOUT\s+ROWID\b`), "SQLite only"},
	{regexp.MustCompile(`(?i)\b(INSERT\s+OR|REPLACE\s+INTO)\b`), "use INSERT … ON CONFLICT"},
	{regexp.MustCompile(`(?i)\bPRAGMA\b`), "SQLite only; the store sets its pragmas"},
	{regexp.MustCompile(`(?i)\bGLOB\b`), "use LIKE"},
	{regexp.MustCompile(`(?i)\b(IFNULL|IIF|DATETIME|JULIANDAY|STRFTIME|UNIXEPOCH|GROUP_CONCAT|RANDOMBLOB|TYPEOF|INSTR|PRINTF|LAST_INSERT_ROWID|CHANGES|TOTAL_CHANGES|JSON_EXTRACT)\s*\(`),
		"a SQLite function; use a portable one, or compute it in Go"},
	{regexp.MustCompile(`(?i)\b(DATE|TIME)\s*\(`), "SQLite's date functions; store times as text, in TimeLayout"},
	{regexp.MustCompile(`(?i)\b(BLOB|DATETIME|TIMESTAMP|REAL|NUMERIC|INTEGER)\b`),
		"use TEXT, BIGINT or BOOLEAN: store bytes as hex or base64, and times as text"},
}

// sqlFiles returns the migrations' and the queries' SQL, by file.
func sqlFiles(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{}
	for _, pattern := range []string{"migrations/*.sql", "*.sql"} {
		names, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range names {
			b, err := os.ReadFile(n)
			if err != nil {
				t.Fatal(err)
			}
			files[n] = string(b)
		}
	}
	if len(files) < 2 {
		t.Fatalf("found only %d SQL files", len(files))
	}
	return files
}

func TestSQLIsPortable(t *testing.T) {
	for name, sql := range sqlFiles(t) {
		for i, line := range strings.Split(sql, "\n") {
			code, _, _ := strings.Cut(line, "--")
			for _, s := range sqliteOnly {
				if m := s.re.FindString(code); m != "" {
					t.Errorf("%s:%d: %q: %s", name, i+1, m, s.note)
				}
			}
		}
	}
}

var migrationFile = regexp.MustCompile(`^(\d{5})_[a-z0-9_]+\.sql$`)

// TestMigrationsOnlyMoveForward checks each migration's name and sections:
// numbered from 1 with no gap, an Up section, and no Down.
func TestMigrationsOnlyMoveForward(t *testing.T) {
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	for i, e := range entries {
		m := migrationFile.FindStringSubmatch(e.Name())
		if m == nil {
			t.Errorf("%s: want a name such as 00001_database.sql", e.Name())
			continue
		}
		if want := fmt.Sprintf("%05d", i+1); m[1] != want {
			t.Errorf("%s: want number %s, the next after the one before it", e.Name(), want)
		}
		b, err := fs.ReadFile(migrations, "migrations/"+e.Name())
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "-- +goose Up") {
			t.Errorf("%s: no -- +goose Up section", e.Name())
		}
		if strings.Contains(string(b), "-- +goose Down") {
			t.Errorf("%s: has a -- +goose Down section; migrations only move forward (ADR-0038)", e.Name())
		}
	}
}

// TestPortabilityCheckCatches checks the patterns themselves.
func TestPortabilityCheckCatches(t *testing.T) {
	bad := []string{
		"id INTEGER PRIMARY KEY AUTOINCREMENT",
		"INSERT OR REPLACE INTO t VALUES (1)",
		"SELECT ifnull(a, b) FROM t",
		"SELECT datetime('now')",
		"SELECT date('now')",
		"payload BLOB NOT NULL",
		"WHERE name GLOB 'a*'",
	}
	for _, sql := range bad {
		caught := false
		for _, s := range sqliteOnly {
			caught = caught || s.re.MatchString(sql)
		}
		if !caught {
			t.Errorf("%q passes the portability check", sql)
		}
	}
	good := []string{
		"seq BIGINT PRIMARY KEY",
		"INSERT INTO t (a) VALUES (1) ON CONFLICT (a) DO NOTHING",
		"SELECT COALESCE(a, b) FROM t",
		"created_at TEXT NOT NULL",
		"ok BOOLEAN NOT NULL",
		"WHERE updated_at > sqlc.arg(since)",
	}
	for _, sql := range good {
		for _, s := range sqliteOnly {
			if s.re.MatchString(sql) {
				t.Errorf("%q fails the portability check: %s", sql, s.note)
			}
		}
	}
}
