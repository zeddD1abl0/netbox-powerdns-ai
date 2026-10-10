package cli

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
)

// TestServeAppliesSettings changes settings with nbpdns settings while
// serve runs, and sees serve apply them within seconds.
func TestServeAppliesSettings(t *testing.T) {
	env := configFile(t, twoGroups)
	env["NBPDNS_NETBOX_URL"], env["NBPDNS_NETBOX_TOKEN"] = "http://127.0.0.1:1", "nbt_abc.def"
	env["NBPDNS_DATABASE_URL"] = "sqlite://" + filepath.Join(t.TempDir(), "nbpdns.db")
	s := startServe(t, env)
	dbEnv := map[string]string{"NBPDNS_DATABASE_URL": env["NBPDNS_DATABASE_URL"]}

	code, stdout, stderr := run(t, dbEnv, "settings", "set", "drift.interval", "15s", "--reason", "a test")
	if code != exitOK || stdout != "Stored 15s for drift.interval.\nA running nbpdns serve applies it within 5s.\n" {
		t.Fatalf("settings set: exit %d: %q\n%s", code, stdout, stderr)
	}
	if code, _, stderr := run(t, dbEnv, "settings", "set", "drift.webhook_delay", "2s"); code != exitOK {
		t.Fatalf("settings set: exit %d:\n%s", code, stderr)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		_, page := s.get("/status?json=1")
		var st struct {
			Schedule struct {
				IntervalSeconds float64 `json:"interval_seconds"`
			} `json:"schedule"`
			Webhooks struct {
				DelaySeconds float64 `json:"delay_seconds"`
			} `json:"webhooks"`
		}
		if err := json.Unmarshal([]byte(page), &st); err != nil {
			t.Fatal(err)
		}
		if st.Schedule.IntervalSeconds == 15 && st.Webhooks.DelaySeconds == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("serve didn't apply the settings within 15s: %s", page)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !regexp.MustCompile(`"msg":"applied the changed settings","keys":\["drift.(interval|webhook_delay)"`).MatchString(s.stderr.String()) {
		t.Errorf("serve didn't log the reload:\n%s", s.stderr)
	}
	if code, page := s.get("/metrics"); code != 200 || !strings.Contains(page, `nbpdns_settings_reloads_total{outcome="applied"}`) {
		t.Errorf("no reload in the metrics:\n%s", page)
	}

	code, stdout, _ = run(t, dbEnv, "settings", "list")
	if code != exitOK || !regexp.MustCompile(`(?m)^drift\.interval +15s +database +15s$`).MatchString(stdout) ||
		!regexp.MustCompile(`(?m)^log\.level +info +default +-$`).MatchString(stdout) || strings.Contains(stdout, "database.url") {
		t.Errorf("settings list: exit %d:\n%s", code, stdout)
	}
	code, stdout, _ = run(t, dbEnv, "settings", "get", "drift.interval")
	if code != exitOK || stdout != "15s\n" {
		t.Errorf("settings get: exit %d: %q", code, stdout)
	}
	code, stdout, _ = run(t, dbEnv, "config", "show")
	if code != exitOK || !regexp.MustCompile(`(?m)^drift\.interval +15s +database$`).MatchString(stdout) {
		t.Errorf("config show doesn't name the database:\n%s", stdout)
	}

	// Pinned by the environment, the stored value is kept, but not used.
	pinned := map[string]string{"NBPDNS_DATABASE_URL": dbEnv["NBPDNS_DATABASE_URL"], "NBPDNS_DRIFT_INTERVAL": "2m"}
	code, stdout, _ = run(t, pinned, "settings", "set", "drift.interval", "20s")
	if code != exitOK || !strings.Contains(stdout, "env NBPDNS_DRIFT_INTERVAL pins drift.interval, so nbpdns uses its value until it's unset there.") {
		t.Errorf("settings set of a pinned key: exit %d: %q", code, stdout)
	}
	code, stdout, _ = run(t, pinned, "settings", "list")
	if !regexp.MustCompile(`(?m)^drift\.interval +2m0s +env NBPDNS_DRIFT_INTERVAL +20s \(pinned\)$`).MatchString(stdout) {
		t.Errorf("settings list with a pinned key: exit %d:\n%s", code, stdout)
	}
	code, stdout, _ = run(t, dbEnv, "settings", "unset", "drift.interval", "--reason", "done")
	if code != exitOK || !strings.HasPrefix(stdout, "Removed the stored value of drift.interval.\n") {
		t.Errorf("settings unset: exit %d: %q", code, stdout)
	}
	if code, stdout, _ = run(t, dbEnv, "settings", "unset", "drift.interval"); stdout != "The database stores no value for drift.interval.\n" {
		t.Errorf("settings unset again: exit %d: %q", code, stdout)
	}

	code, stdout, _ = run(t, dbEnv, "audit", "list", "--target", "setting:drift.interval", "-o", "json")
	var events []struct {
		Actor, Action, Reason string
	}
	if err := json.Unmarshal([]byte(stdout), &events); err != nil || code != exitOK || len(events) != 3 {
		t.Fatalf("audit list: exit %d, %v:\n%s", code, err, stdout)
	}
	if user := audit.OSUser().String(); events[0].Actor != user || events[0].Reason != "a test" ||
		events[2].Action != string(audit.SettingUnset) || events[2].Reason != "done" {
		t.Errorf("events = %+v, want three, by %s", events, user)
	}
	if code := s.stop(); code != exitOK {
		t.Errorf("exit %d:\n%s", code, s.stderr)
	}
}

func TestSettingsErrors(t *testing.T) {
	db := filepath.Join(t.TempDir(), "nbpdns.db")
	env := map[string]string{"NBPDNS_DATABASE_URL": "sqlite://" + db}
	if code, _, stderr := run(t, env, "db", "migrate"); code != exitOK {
		t.Fatalf("db migrate: exit %d:\n%s", code, stderr)
	}
	for _, tt := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"settings", "set", "database.url", "sqlite:///x.db"}, exitUsage,
			"database.url is a bootstrap key, which only the config file, NBPDNS_DATABASE_URL or --database-url sets"},
		{[]string{"settings", "set", "drift.interval", "1s"}, exitUsage, "drift.interval: 1s is shorter than 10s"},
		{[]string{"settings", "set", "drift.intervall", "1m"}, exitUsage, "there's no key drift.intervall"},
		{[]string{"settings", "set", "netbox.token", "nbt_abc.s3cret"}, exitUsage, "netbox.token is a secret, which the database doesn't store yet"},
		{[]string{"settings", "unset", "server.listen"}, exitUsage, "server.listen is a bootstrap key"},
		{[]string{"settings", "get", "server.listen"}, exitError, "server.listen is a bootstrap key"},
		{[]string{"settings", "set", "drift.interval"}, exitUsage, "accepts 2 arg(s)"},
	} {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			code, _, stderr := run(t, env, tt.args...)
			if code != tt.code || !strings.Contains(stderr, tt.want) || strings.Contains(stderr, "s3cret") {
				t.Errorf("exit %d, want %d with %q:\n%s", code, tt.code, tt.want, stderr)
			}
		})
	}
}

// TestReconfigure checks that a refresh makes new clients when, and only
// when, their settings change.
func TestReconfigure(t *testing.T) {
	s := testSession(t)
	s.cfg.NetBox = config.NetBoxConfig{URL: "http://127.0.0.1:1", Token: config.NewSecret("nbt_a.b"), Timeout: time.Second, PageSize: 10, Concurrency: 1}
	groups := []config.Group{{Name: "site-a", Primary: config.Primary{URL: "http://127.0.0.1:1", APIKey: config.NewSecret("k"), ServerID: "localhost"}}}
	c, err := s.newClients(t.Context(), groups)
	if err != nil {
		t.Fatal(err)
	}
	defer c.close()
	nb, pdns := c.netbox, c.groups[0].c

	next := *s.cfg
	next.Drift.Interval = time.Minute
	c.reconfigure(t.Context(), &next)
	if c.netbox != nb || c.groups[0].c != pdns || s.cfg != &next {
		t.Error("a change of drift.interval made new clients, or left the old configuration")
	}

	timeouts := next
	timeouts.NetBox.Timeout, timeouts.PowerDNS.Timeout = 2*time.Second, 9*time.Second
	c.reconfigure(t.Context(), &timeouts)
	if c.netbox == nb || c.groups[0].c == pdns || len(c.groups) != 1 || c.groups[0].cfg.Name != "site-a" {
		t.Error("a change of the timeouts kept the old clients")
	}
}
