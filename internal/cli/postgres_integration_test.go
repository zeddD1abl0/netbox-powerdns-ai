//go:build integration

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/lab"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/secrets"
)

// TestCLIOnPostgres runs each command that uses the database on a database
// in the lab's PostgreSQL, with the URL from NBPDNS_DATABASE_URL_FILE
// (ADR-0046), and checks that the URL's password shows nowhere.
func TestCLIOnPostgres(t *testing.T) {
	where := lab.NewPostgresDatabase(t)
	shown := strings.Replace(where, lab.PostgresPassword, "xxxxx", 1)
	dir := t.TempDir()
	urlFile, apiKey, newKey := filepath.Join(dir, "database-url"), filepath.Join(dir, "api-key"), filepath.Join(dir, "new.key")
	token := filepath.Join(dir, "token")
	for path, content := range map[string]string{
		urlFile: where + "\n", apiKey: "pdns-s3cret\n", newKey: secrets.NewMasterKey().Base64() + "\n", token: "nbt_abc.s3cret\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_, keyPath := keyFile(t)
	env := map[string]string{"NBPDNS_DATABASE_URL_FILE": urlFile, "NBPDNS_DATABASE_MASTER_KEY_FILE": keyPath}
	var all strings.Builder // everything that the commands wrote
	cmd := func(want int, args ...string) (string, string) {
		t.Helper()
		code, stdout, stderr := run(t, env, args...)
		all.WriteString(stdout + stderr)
		if code != want {
			t.Fatalf("%s: exit %d, want %d:\n%s%s", strings.Join(args, " "), code, want, stdout, stderr)
		}
		return stdout, stderr
	}
	match := func(what, re, out string) {
		t.Helper()
		if !regexp.MustCompile(re).MatchString(out) {
			t.Errorf("%s: no match for %s:\n%s", what, re, out)
		}
	}

	// An empty database lacks every migration, and isn't made into one.
	stdout, _ := cmd(exitOK, "db", "status")
	match("db status of an empty database", `(?m)^Schema:   version 0 of \d+; run nbpdns db migrate`, stdout)
	cmd(exitError, "settings", "list")

	stdout, _ = cmd(exitOK, "db", "migrate")
	match("db migrate", `(?m)^Applied migration 1, database\.$`, stdout)
	match("db migrate", `(?m)^The database, `+regexp.QuoteMeta(shown)+`, is up to date, at version \d+\.$`, stdout)
	stdout, _ = cmd(exitOK, "db", "status")
	match("db status", `(?m)^Database: PostgreSQL 18\.\d+, `+regexp.QuoteMeta(shown)+`, unencrypted$`, stdout)
	match("db status", `(?m)^Schema:   version \d+, up to date$`, stdout)
	stdout, _ = cmd(exitOK, "db", "status", "-o", "json")
	var r dbStatus
	if err := json.Unmarshal([]byte(stdout), &r); err != nil {
		t.Fatal(err)
	}
	if r.Backend != "postgresql" || r.URL != shown || r.Path != "" || r.TLS == nil || *r.TLS || r.ID == "" || r.Pending {
		t.Errorf("db status -o json = %+v", r)
	}

	cmd(exitOK, "settings", "set", "drift.interval", "2m", "--reason", "test")
	cmd(exitOK, "settings", "set", "netbox.token", "--from-file", token, "--reason", "test")
	stdout, _ = cmd(exitOK, "settings", "list")
	match("settings list", `(?m)^drift\.interval +2m0s +database`, stdout)
	cmd(exitOK, "groups", "add", "site-c", "--url", "http://127.0.0.1:1", "--views", "_default_",
		"--api-key-file", apiKey, "--no-check", "--reason", "a new site")
	stdout, _ = cmd(exitOK, "groups", "list")
	match("groups list", `(?m)^site-c +cli +http://127\.0\.0\.1:1 `, stdout)
	stdout, _ = cmd(exitOK, "secrets", "rotate-master-key", "--new-key-file", newKey, "--reason", "test")
	match("secrets rotate-master-key", `Re-wrapped 2 secrets`, stdout)
	env["NBPDNS_DATABASE_MASTER_KEY_FILE"] = newKey
	stdout, _ = cmd(exitOK, "settings", "get", "netbox.token")
	match("settings get, after the rotation", `\[redacted\]`, stdout)

	stdout, _ = cmd(exitOK, "audit", "list")
	for _, action := range []string{"setting.set", "group.created", "master_key.rotated"} {
		match("audit list", `(?m) `+regexp.QuoteMeta(action)+` `, stdout)
	}
	stdout, _ = cmd(exitOK, "audit", "verify")
	match("audit verify", `^The audit trail is intact: \d+ events`, stdout)

	stdout, _ = cmd(exitOK, "config", "show")
	match("config show", `(?m)^database\.url +`+regexp.QuoteMeta(shown)+` +env NBPDNS_DATABASE_URL_FILE$`, stdout)

	if strings.Contains(all.String(), lab.PostgresPassword) {
		t.Errorf("a command showed the database's password:\n%s", all.String())
	}
}

// TestServeOnPostgres runs serve on PostgreSQL, checks that /status says so
// without the password, and that a second serve, or db migrate, refuses to
// run on the same database while it does (ADR-0046).
func TestServeOnPostgres(t *testing.T) {
	where := lab.NewPostgresDatabase(t)
	shown := strings.Replace(where, lab.PostgresPassword, "xxxxx", 1)
	env := configFile(t, twoGroups)
	env["NBPDNS_NETBOX_URL"], env["NBPDNS_NETBOX_TOKEN"] = "http://127.0.0.1:1", "nbt_abc.def"
	env["NBPDNS_DATABASE_URL"] = where
	first := startServe(t, env)
	first.waitReady(30 * time.Second)
	id := loggedDatabaseID(t, first.stderr.String())
	var pages strings.Builder

	code, page := first.get("/status")
	pages.WriteString(page)
	if want := "Database: PostgreSQL, " + shown + ", unencrypted\n"; code != http.StatusOK || !strings.Contains(page, want) {
		t.Errorf("/status: %d, without %q:\n%s", code, want, page)
	}
	for _, path := range []string{"/status?json=1", "/api/status"} {
		var code int
		var body string
		if path == "/api/status" {
			code, body = first.api(path) // checked against the API's spec
		} else {
			code, body = first.get(path)
		}
		pages.WriteString(body)
		var st struct {
			Database struct {
				Backend, URL, Path string
				TLS                *bool
			}
		}
		if err := json.Unmarshal([]byte(body), &st); err != nil || code != http.StatusOK {
			t.Fatalf("%s: %d, %v:\n%s", path, code, err, body)
		}
		if d := st.Database; d.Backend != "postgresql" || d.URL != shown || d.Path != "" || d.TLS == nil || *d.TLS {
			t.Errorf("%s: database = %+v", path, d)
		}
	}

	second := with(env, "NBPDNS_SERVER_LISTEN", "127.0.0.1:0")
	host, _ := os.Hostname()
	code, _, stderr := run(t, second, "serve")
	want := fmt.Sprintf("another nbpdns holds the database's lock: host %s, PID %d, since ", host, os.Getpid())
	if code != exitError || !strings.Contains(stderr, want) {
		t.Errorf("a second serve: exit %d, want %d with %q:\n%s", code, exitError, want, stderr)
	}
	pages.WriteString(stderr)
	code, _, stderr = run(t, second, "db", "migrate")
	if code != exitError || !strings.Contains(stderr, "another nbpdns holds the database's lock") {
		t.Errorf("db migrate while serve runs: exit %d:\n%s", code, stderr)
	}
	if code := first.stop(); code != exitOK {
		t.Fatalf("exit %d:\n%s", code, first.stderr)
	}

	// The lock went with the first serve's session.
	again := startServe(t, env)
	if got := loggedDatabaseID(t, again.stderr.String()); got != id {
		t.Errorf("after a restart, the database's ID is %s, want %s", got, id)
	}
	for name, out := range map[string]string{"/status and the second serve": pages.String(),
		"the first serve's log": first.stderr.String(), "the restarted serve's log": again.stderr.String()} {
		if strings.Contains(out, lab.PostgresPassword) {
			t.Errorf("%s shows the database's password:\n%s", name, out)
		}
	}
	if !strings.Contains(first.stderr.String(), "the connection to PostgreSQL isn't encrypted") {
		t.Errorf("serve didn't warn of the unencrypted connection:\n%s", first.stderr)
	}
}
