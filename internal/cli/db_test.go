package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDBCommands(t *testing.T) {
	db := filepath.Join(t.TempDir(), "nbpdns.db")
	env := map[string]string{"NBPDNS_DATABASE_URL": "sqlite://" + db}

	// No database yet: status says how to make one, and makes nothing.
	code, _, stderr := run(t, env, "db", "status")
	if code != exitError || !strings.Contains(stderr, "there's no database at "+db+"; nbpdns serve, or nbpdns db migrate, creates it") {
		t.Errorf("db status without a database: exit %d:\n%s", code, stderr)
	}
	if _, err := os.Stat(db); err == nil {
		t.Error("db status made the database")
	}

	code, stdout, stderr := run(t, env, "db", "migrate")
	if code != exitOK || !strings.Contains(stdout, "Applied migration 1, database.\n") ||
		!strings.Contains(stdout, "The database, "+db+", is up to date, at version ") {
		t.Errorf("db migrate: exit %d:\n%s%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, `"msg":"applied a database migration"`) {
		t.Errorf("db migrate logged no migration:\n%s", stderr)
	}
	code, stdout, stderr = run(t, env, "db", "migrate")
	if code != exitOK || strings.Contains(stdout, "Applied") {
		t.Errorf("db migrate again: exit %d, want nothing applied:\n%s%s", code, stdout, stderr)
	}

	code, stdout, stderr = run(t, env, "db", "status")
	for _, re := range []string{
		`(?m)^Database: ` + regexp.QuoteMeta(db) + `$`,
		`(?m)^ID:       \S+, created \S+ by nbpdns \S+$`,
		`(?m)^Schema:   version \d+, up to date$`,
		`(?m)^VERSION +NAME +APPLIED$`,
		`(?m)^1 +database +\d{4}-\d\d-\d\dT`,
	} {
		if !regexp.MustCompile(re).MatchString(stdout) {
			t.Errorf("db status: exit %d, no match for %s:\n%s%s", code, re, stdout, stderr)
		}
	}

	code, stdout, stderr = run(t, env, "db", "status", "-o", "json")
	var r dbStatus
	if err := json.Unmarshal([]byte(stdout), &r); err != nil || code != exitOK {
		t.Fatalf("db status -o json: exit %d, %v:\n%s%s", code, err, stdout, stderr)
	}
	if r.Path != db || r.ID == "" || r.Pending || r.Version != r.Latest || r.Migrations[0].Applied == nil {
		t.Errorf("db status -o json = %+v", r)
	}
}

// TestServeOneInstancePerDatabase starts a second serve on a database that
// one is using, and sees it refuse; then restarts the first, which keeps the
// database it made.
func TestServeOneInstancePerDatabase(t *testing.T) {
	env := configFile(t, twoGroups)
	env["NBPDNS_NETBOX_URL"], env["NBPDNS_NETBOX_TOKEN"] = "http://127.0.0.1:1", "nbt_abc.def"
	env["NBPDNS_DATABASE_URL"] = "sqlite://" + filepath.Join(t.TempDir(), "nbpdns.db")
	first := startServe(t, env)
	id := loggedDatabaseID(t, first.stderr.String())

	second := map[string]string{}
	for k, v := range env {
		second[k] = v
	}
	second["NBPDNS_SERVER_LISTEN"] = "127.0.0.1:0"
	code, _, stderr := run(t, second, "serve")
	want := fmt.Sprintf("another nbpdns holds the database's lock, %s.lock: host ", strings.TrimPrefix(env["NBPDNS_DATABASE_URL"], "sqlite://"))
	if code != exitError || !strings.Contains(stderr, want) || !strings.Contains(stderr, fmt.Sprintf(", PID %d, since ", os.Getpid())) {
		t.Errorf("a second serve: exit %d, want %d with %q:\n%s", code, exitError, want, stderr)
	}
	code, _, stderr = run(t, second, "db", "migrate")
	if code != exitError || !strings.Contains(stderr, "another nbpdns holds the database's lock") {
		t.Errorf("db migrate while serve runs: exit %d:\n%s", code, stderr)
	}
	if code := first.stop(); code != exitOK {
		t.Fatalf("exit %d:\n%s", code, first.stderr)
	}

	again := startServe(t, env)
	if got := loggedDatabaseID(t, again.stderr.String()); got != id {
		t.Errorf("after a restart, the database's ID is %s, want %s", got, id)
	}
	if strings.Contains(again.stderr.String(), "applied a database migration") {
		t.Errorf("a restart migrated the database again:\n%s", again.stderr)
	}
}

// loggedDatabaseID returns the database's ID from serve's log.
func loggedDatabaseID(t *testing.T, log string) string {
	t.Helper()
	for l := range strings.Lines(log) {
		var rec struct {
			Msg string `json:"msg"`
			ID  string `json:"database_id"`
		}
		if json.Unmarshal([]byte(l), &rec) == nil && rec.Msg == "opened the database" && rec.ID != "" {
			return rec.ID
		}
	}
	t.Fatalf("serve didn't log the database's ID:\n%s", log)
	return ""
}
