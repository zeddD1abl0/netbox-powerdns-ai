package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
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

// TestPostgresURLIsSecret checks that a PostgreSQL URL's password shows
// nowhere: not in config show, which masks it, nor in the error of a
// connection that fails, nor in the warning of config show, which carries on
// without the database (ADR-0046).
func TestPostgresURLIsSecret(t *testing.T) {
	const pw = "s3cret-pw"
	where := "postgres://nbpdns:" + pw + "@127.0.0.1:1/nbpdns?sslmode=disable&sslpassword=" + pw
	shown := "postgres://nbpdns:xxxxx@127.0.0.1:1/nbpdns?sslmode=disable&sslpassword=xxxxx"
	env := map[string]string{"NBPDNS_DATABASE_URL": where}

	code, stdout, stderr := run(t, env, "config", "show")
	if code != exitOK || !regexp.MustCompile(`(?m)^database\.url +`+regexp.QuoteMeta(shown)+` +env NBPDNS_DATABASE_URL$`).MatchString(stdout) {
		t.Errorf("config show: exit %d, want database.url as %s:\n%s%s", code, shown, stdout, stderr)
	}
	if !strings.Contains(stderr, "couldn't read the settings that the database stores") || !strings.Contains(stderr, "connection refused") {
		t.Errorf("config show without the database: no warning:\n%s", stderr)
	}
	all := stdout + stderr
	for _, args := range [][]string{{"db", "status"}, {"db", "migrate"}, {"serve"}, {"settings", "list"}} {
		code, stdout, stderr := run(t, with(env, "NBPDNS_SERVER_LISTEN", "127.0.0.1:0"), args...)
		if code != exitError || !strings.Contains(stderr, "opening the database: failed to connect to `user=nbpdns database=nbpdns`") {
			t.Errorf("%s: exit %d, want %d, failing to connect:\n%s", strings.Join(args, " "), code, exitError, stderr)
		}
		all += stdout + stderr
	}
	if strings.Contains(all, pw) {
		t.Errorf("the output shows the password:\n%s", all)
	}
}

// TestFingerprintLeavesOutThePassword checks that the configuration's
// fingerprint, which serve records in the audit trail at each start, covers
// database.url without its password: a change of password alone doesn't
// change it, and a change of database does (ADR-0046).
func TestFingerprintLeavesOutThePassword(t *testing.T) {
	fingerprint := func(url string) string {
		t.Helper()
		t.Setenv("NBPDNS_DATABASE_URL", url)
		cfg, settings, err := config.NewLoader().Load()
		if err != nil {
			t.Fatal(err)
		}
		return (&session{cfg: cfg, settings: settings}).configFingerprint()
	}
	one := fingerprint("postgres://nbpdns:first-s3cret@db.example.com/nbpdns")
	if two := fingerprint("postgres://nbpdns:second-s3cret@db.example.com/nbpdns"); two != one {
		t.Errorf("a change of password changed the fingerprint, from %s to %s", one, two)
	}
	if other := fingerprint("postgres://nbpdns:first-s3cret@db2.example.com/nbpdns"); other == one {
		t.Error("a change of database didn't change the fingerprint")
	}
}
