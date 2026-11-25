package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestAuditVerifyCounts checks how audit verify counts the events of an
// intact trail, as each change adds one: none, one, then several.
func TestAuditVerifyCounts(t *testing.T) {
	env := map[string]string{"NBPDNS_DATABASE_URL": "sqlite://" + filepath.Join(t.TempDir(), "nbpdns.db")}
	if code, _, stderr := run(t, env, "db", "migrate"); code != exitOK {
		t.Fatalf("db migrate: exit %d:\n%s", code, stderr)
	}
	tests := []struct {
		name string
		// changes is how many settings to set before audit verify runs.
		changes int
		want    string
	}{
		{name: "no events", want: `^The audit trail is empty\.\n$`},
		{name: "one event", changes: 1, want: `^The audit trail is intact: 1 event, event 1, at \S+Z\.\n$`},
		{name: "two events", changes: 1, want: `^The audit trail is intact: 2 events, from event 1, at \S+Z, to event 2, at \S+Z\.\n$`},
	}
	interval := 1
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for range tc.changes {
				interval++
				if code, _, stderr := run(t, env, "settings", "set", "drift.interval", fmt.Sprintf("%dm", interval), "--reason", "test"); code != exitOK {
					t.Fatalf("settings set: exit %d:\n%s", code, stderr)
				}
			}
			code, stdout, stderr := run(t, env, "audit", "verify")
			if code != exitOK || !regexp.MustCompile(tc.want).MatchString(stdout) {
				t.Errorf("audit verify: exit %d, want %d and a match for %s:\n%s%s", code, exitOK, tc.want, stdout, stderr)
			}
		})
	}
}

// TestServeRecordsItsStartAndStop runs serve, stops it, and reads its two
// events back, then tampers with the trail and sees audit verify fail.
func TestServeRecordsItsStartAndStop(t *testing.T) {
	db := filepath.Join(t.TempDir(), "nbpdns.db")
	env := configFile(t, twoGroups)
	env["NBPDNS_NETBOX_URL"], env["NBPDNS_NETBOX_TOKEN"] = "http://127.0.0.1:1", "nbt_abc.def"
	env["NBPDNS_DATABASE_URL"] = "sqlite://" + db
	s := startServe(t, env)
	if code := s.stop(); code != exitOK {
		t.Fatalf("exit %d:\n%s", code, s.stderr)
	}
	dbEnv := map[string]string{"NBPDNS_DATABASE_URL": "sqlite://" + db}

	code, stdout, stderr := run(t, dbEnv, "audit", "list")
	for _, re := range []string{
		`(?m)^SEQ +TIME +ACTOR +ACTION +TARGET +OUTCOME +REASON$`,
		// The config file's groups, mirrored as serve starts.
		`(?m)^1 +\S+ +file:\S+ +group\.created +server-group:site-a +success +-$`,
		`(?m)^2 +\S+ +file:\S+ +group\.created +server-group:site-b +success +-$`,
		`(?m)^3 +\S+ +system +service\.started +service:\S+ +success +-$`,
		`(?m)^4 +\S+ +system +service\.stopped +service:\S+ +success +-$`,
	} {
		if !regexp.MustCompile(re).MatchString(stdout) {
			t.Errorf("audit list: exit %d, no match for %s:\n%s%s", code, re, stdout, stderr)
		}
	}

	code, stdout, stderr = run(t, dbEnv, "audit", "list", "-o", "json", "--target", "service")
	var events []struct {
		Seq       int64          `json:"seq"`
		Action    string         `json:"action"`
		After     map[string]any `json:"after"`
		TraceID   string         `json:"trace_id"`
		RequestID string         `json:"request_id"`
		Hash      string         `json:"hash"`
	}
	if err := json.Unmarshal([]byte(stdout), &events); err != nil || code != exitOK || len(events) != 2 {
		t.Fatalf("audit list -o json: exit %d, %v:\n%s%s", code, err, stdout, stderr)
	}
	fp, _ := events[0].After["config_fingerprint"].(string)
	if !strings.HasPrefix(fp, "sha384:") || len(fp) != len("sha384:")+96 || events[0].After["version"] == nil {
		t.Errorf("service.started's after = %v, want the version and a SHA-384 fingerprint", events[0].After)
	}
	if _, ok := events[1].After["uptime_seconds"]; !ok || events[1].After["error"] != nil {
		t.Errorf("service.stopped's after = %v, want its uptime, and no error", events[1].After)
	}
	if len(events[0].TraceID) != 32 || events[0].RequestID == "" || len(events[0].Hash) != 96 {
		t.Errorf("service.started = %+v, want serve's trace and request IDs, and a hash", events[0])
	}

	code, stdout, _ = run(t, dbEnv, "audit", "list", "--action", "service.stopped", "--target", "service")
	if code != exitOK || strings.Count(stdout, "\n") != 2 || !strings.Contains(stdout, "service.stopped") {
		t.Errorf("audit list --action service.stopped: exit %d:\n%s", code, stdout)
	}
	code, stdout, _ = run(t, dbEnv, "audit", "list", "--since", "1h", "--actor", "os-user:nobody")
	if code != exitOK || strings.Count(stdout, "\n") != 1 {
		t.Errorf("audit list for an actor with no events: exit %d, want only the header:\n%s", code, stdout)
	}

	code, stdout, stderr = run(t, dbEnv, "audit", "verify")
	if code != exitOK || !regexp.MustCompile(`^The audit trail is intact: 4 events, from event 1, at \S+, to event 4, at \S+\.\n$`).MatchString(stdout) {
		t.Errorf("audit verify: exit %d:\n%s%s", code, stdout, stderr)
	}

	raw := rawSQLite(t, db)
	if _, err := raw.ExecContext(t.Context(), "UPDATE audit_events SET actor = 'os-user:mallory' WHERE seq = 3"); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = run(t, dbEnv, "audit", "verify")
	if code != exitError || !strings.Contains(stderr, "the audit trail is broken at event 3: its contents don't match its hash") {
		t.Errorf("audit verify of an edited trail: exit %d:\n%s", code, stderr)
	}
}

func TestAuditListErrors(t *testing.T) {
	db := filepath.Join(t.TempDir(), "nbpdns.db")
	env := map[string]string{"NBPDNS_DATABASE_URL": "sqlite://" + db}
	for _, tt := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"audit", "list", "--since", "yesterday"}, exitUsage, `--since "yesterday" isn't a time in RFC 3339`},
		{[]string{"audit", "list", "--until", "-1h"}, exitUsage, `--until "-1h" isn't a time`},
		{[]string{"audit", "list", "--limit", "0"}, exitUsage, "--limit 0 isn't at least 1"},
		{[]string{"audit", "verify"}, exitError, "there's no database at " + db},
	} {
		t.Run(strings.Join(tt.args[1:], " "), func(t *testing.T) {
			code, _, stderr := run(t, env, tt.args...)
			if code != tt.code || !strings.Contains(stderr, tt.want) {
				t.Errorf("exit %d, want %d with %q:\n%s", code, tt.code, tt.want, stderr)
			}
		})
	}
}
