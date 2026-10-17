package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
)

// groupsEnv returns the environment of a migrated database with a master
// key, and a file that holds an API key.
func groupsEnv(t *testing.T) (map[string]string, string) {
	t.Helper()
	db := filepath.Join(t.TempDir(), "nbpdns.db")
	_, keyPath := keyFile(t)
	env := map[string]string{"NBPDNS_DATABASE_URL": "sqlite://" + db, "NBPDNS_DATABASE_MASTER_KEY_FILE": keyPath}
	if code, _, stderr := run(t, env, "db", "migrate"); code != exitOK {
		t.Fatalf("db migrate: exit %d:\n%s", code, stderr)
	}
	apiKey := filepath.Join(t.TempDir(), "api-key")
	if err := os.WriteFile(apiKey, []byte("pdns-s3cret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return env, apiKey
}

// with returns env with more variables.
func with(env map[string]string, kv ...string) map[string]string {
	out := map[string]string{}
	for k, v := range env {
		out[k] = v
	}
	for i := 0; i+1 < len(kv); i += 2 {
		out[kv[i]] = kv[i+1]
	}
	return out
}

func TestGroupsCommands(t *testing.T) {
	env, apiKey := groupsEnv(t)
	unreachable := "http://127.0.0.1:1"
	if code, stdout, stderr := run(t, env, "groups", "list"); code != exitOK || stdout != "The database stores no server groups.\n" {
		t.Errorf("groups list: exit %d: %q\n%s", code, stdout, stderr)
	}

	// The primary must answer, unless --no-check says otherwise.
	code, stdout, stderr := run(t, env, "groups", "add", "site-c", "--url", unreachable, "--views", "_default_", "--api-key-file", apiKey)
	if code != exitError || !strings.Contains(stderr, "server group site-c's primary failed a check, so nothing changed; fix it, or pass --no-check") ||
		!regexp.MustCompile(`(?m)^site-c +server +failed +`).MatchString(stdout) {
		t.Errorf("groups add of an unreachable primary: exit %d:\n%s%s", code, stdout, stderr)
	}
	code, stdout, stderr = run(t, env, "groups", "add", "site-c", "--url", unreachable, "--views", "_default_,internal",
		"--zone-policies", "legacy.example.com=ignore", "--api-key-file", apiKey, "--no-check", "--reason", "a new site")
	if code != exitOK || stdout != "Added server group site-c, its API key sealed with the master key.\nA running nbpdns serve compares it within 5s.\n" {
		t.Fatalf("groups add: exit %d: %q\n%s", code, stdout, stderr)
	}
	// The key from the environment.
	if code, _, stderr := run(t, with(env, config.GroupAPIKeyEnv, "pdns-s3cret"), "groups", "add", "site-d", "--url", unreachable,
		"--views", "_default_", "--no-check"); code != exitOK {
		t.Fatalf("groups add with the key in the environment: exit %d:\n%s", code, stderr)
	}
	code, stdout, _ = run(t, env, "groups", "list")
	if code != exitOK || !regexp.MustCompile(`(?m)^site-c +cli +http://127\.0\.0\.1:1 +_default_,internal +report +\S+ +os-user:\S+$`).MatchString(stdout) {
		t.Errorf("groups list: exit %d:\n%s", code, stdout)
	}
	code, stdout, _ = run(t, env, "groups", "list", "-o", "json")
	var listed []storedGroupOut
	if err := json.Unmarshal([]byte(stdout), &listed); err != nil || code != exitOK || len(listed) != 2 {
		t.Fatalf("groups list -o json: exit %d, %v:\n%s", code, err, stdout)
	}
	if g := listed[0]; g.Name != "site-c" || g.ManagedBy != config.ManagedByCLI || g.Primary.APIKey != config.Redacted ||
		g.ZonePolicies["legacy.example.com."] != config.PolicyIgnore || g.Primary.ServerID != "localhost" {
		t.Errorf("site-c = %+v", g)
	}

	for _, tt := range []struct {
		env  map[string]string
		args []string
		code int
		want string
	}{
		{env, []string{"groups", "add", "Site_C", "--url", unreachable, "--views", "v"}, exitUsage, `the group's name: "Site_C" isn't lowercase`},
		{env, []string{"groups", "add", "site-e", "--views", "v", "--api-key-file", apiKey}, exitUsage, "--url is required"},
		{env, []string{"groups", "add", "site-e", "--url", unreachable, "--views", "v"}, exitUsage,
			"give the group's API key with --api-key-file, or in NBPDNS_GROUP_API_KEY"},
		{env, []string{"groups", "add", "site-e", "--url", "ftp://x", "--views", "v", "--api-key-file", apiKey}, exitUsage,
			"server group site-e: primary.url: "},
		{env, []string{"groups", "add", "site-e", "--url", unreachable, "--views", "v", "--api-key-file", apiKey, "--cert-file", "c.pem"},
			exitUsage, "primary.cert_file and primary.key_file go together"},
		{env, []string{"groups", "add", "site-e", "--url", unreachable, "--views", "v", "--zone-policies", "x.example"}, exitUsage,
			`--zone-policies: "x.example" isn't zone=policy`},
		{env, []string{"groups", "add", "site-c", "--url", unreachable, "--views", "v", "--api-key-file", apiKey}, exitError,
			"server group site-c exists, managed by cli"},
		{map[string]string{"NBPDNS_DATABASE_URL": env["NBPDNS_DATABASE_URL"]}, []string{"groups", "add", "site-e", "--url", unreachable,
			"--views", "v", "--api-key-file", apiKey}, exitError, "database.master_key, which isn't set"},
		{env, []string{"groups", "set", "site-c"}, exitUsage, "give a field to change"},
		{env, []string{"groups", "set", "nothing", "--views", "v"}, exitError, "no server group is named nothing"},
		{env, []string{"groups", "set", "site-c", "--drift-policy", "loud"}, exitUsage, `drift_policy: "loud" isn't a drift policy`},
		{env, []string{"groups", "rename", "site-c", "Site_Z"}, exitUsage, `the new name: "Site_Z" isn't lowercase`},
		{env, []string{"groups", "rename", "site-c", "site-d"}, exitError, "server group site-d exists"},
		{env, []string{"groups", "remove", "nothing"}, exitError, "no server group is named nothing"},
	} {
		t.Run(strings.Join(tt.args[1:], " "), func(t *testing.T) {
			code, _, stderr := run(t, tt.env, tt.args...)
			if code != tt.code || !strings.Contains(stderr, tt.want) || strings.Contains(stderr, "s3cret") {
				t.Errorf("exit %d, want %d with %q:\n%s", code, tt.code, tt.want, stderr)
			}
		})
	}

	// A change that doesn't touch how nbpdns reaches the primary isn't
	// checked; one that does is.
	code, stdout, stderr = run(t, env, "groups", "set", "site-c", "--views", "internal", "--zone-policies", "")
	if code != exitOK || stdout != "Changed views, zone_policies of server group site-c.\nA running nbpdns serve applies it within 5s.\n" {
		t.Errorf("groups set of the views: exit %d: %q\n%s", code, stdout, stderr)
	}
	if code, _, stderr := run(t, env, "groups", "set", "site-c", "--url", "http://127.0.0.1:2"); code != exitError ||
		!strings.Contains(stderr, "failed a check") {
		t.Errorf("groups set of an unreachable URL: exit %d:\n%s", code, stderr)
	}
	if code, stdout, stderr := run(t, env, "groups", "set", "site-c", "--url", "http://127.0.0.1:2", "--no-check"); code != exitOK ||
		!strings.HasPrefix(stdout, "Changed primary.url of server group site-c.\n") {
		t.Errorf("groups set --no-check: exit %d: %q\n%s", code, stdout, stderr)
	}
	// The same key changes nothing, so it isn't checked.
	if code, stdout, stderr := run(t, with(env, config.GroupAPIKeyEnv, "pdns-s3cret"), "groups", "set", "site-c"); code != exitOK ||
		stdout != "Server group site-c already has those values.\n" {
		t.Errorf("groups set of the same key: exit %d: %q\n%s", code, stdout, stderr)
	}

	code, stdout, stderr = run(t, env, "groups", "rename", "site-c", "site-z", "--reason", "merged")
	if code != exitOK || !strings.HasPrefix(stdout, "Renamed server group site-c to site-z, with its drift history.\n") {
		t.Errorf("groups rename: exit %d: %q\n%s", code, stdout, stderr)
	}
	code, stdout, stderr = run(t, env, "groups", "remove", "site-z")
	if code != exitOK || !strings.HasPrefix(stdout, "Removed server group site-z. Its drift history is kept") {
		t.Errorf("groups remove: exit %d: %q\n%s", code, stdout, stderr)
	}

	code, stdout, _ = run(t, env, "audit", "list", "--target", "server-group", "-o", "json")
	var events []struct {
		Action, Actor, Reason, Target string
		After                         map[string]any
	}
	if err := json.Unmarshal([]byte(stdout), &events); err != nil || code != exitOK {
		t.Fatalf("audit list: exit %d, %v:\n%s", code, err, stdout)
	}
	var got []string
	for _, e := range events {
		got = append(got, e.Action+" "+e.Target)
	}
	want := "group.created server-group:site-c,group.created server-group:site-d,group.changed server-group:site-c," +
		"group.changed server-group:site-c,group.renamed server-group:site-z,group.removed server-group:site-z"
	if strings.Join(got, ",") != want {
		t.Errorf("events = %s, want %s", strings.Join(got, ","), want)
	}
	if e := events[0]; e.Actor != audit.OSUser().String() || e.Reason != "a new site" || e.After["check"] != store.CheckSkipped {
		t.Errorf("site-c's creation = %+v", e)
	}
	// No key reaches the database's files unsealed.
	db := strings.TrimPrefix(env["NBPDNS_DATABASE_URL"], "sqlite://")
	for _, f := range []string{db, db + "-wal"} {
		b, err := os.ReadFile(f)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if bytes.Contains(b, []byte("s3cret")) {
			t.Errorf("%s holds an API key unsealed", f)
		}
	}

	// The master key's rotation re-wraps the groups' keys.
	_, otherPath := keyFile(t)
	if code, stdout, stderr := run(t, env, "secrets", "rotate-master-key", "--new-key-file", otherPath); code != exitOK ||
		!strings.HasPrefix(stdout, "Re-wrapped 1 secret with master key ") {
		t.Errorf("rotate-master-key: exit %d: %q\n%s", code, stdout, stderr)
	}
	rotated := with(env, "NBPDNS_DATABASE_MASTER_KEY_FILE", otherPath)
	if code, stdout, stderr := run(t, rotated, "groups", "set", "site-d", "--drift-policy", "ignore"); code != exitOK ||
		!strings.HasPrefix(stdout, "Changed drift_policy of server group site-d.") {
		t.Errorf("groups set after the rotation: exit %d: %q\n%s", code, stdout, stderr)
	}
}

func TestGroupsAndTheConfigFile(t *testing.T) {
	env, apiKey := groupsEnv(t)
	file := with(configFile(t, twoGroups), "NBPDNS_DATABASE_URL", env["NBPDNS_DATABASE_URL"],
		"NBPDNS_DATABASE_MASTER_KEY_FILE", env["NBPDNS_DATABASE_MASTER_KEY_FILE"])
	if code, _, stderr := run(t, file, "groups", "add", "site-a", "--url", "http://127.0.0.1:1", "--views", "v",
		"--api-key-file", apiKey, "--no-check"); code != exitError || !strings.Contains(stderr, "declares a server group named site-a") {
		t.Errorf("groups add of a name the config file declares: exit %d:\n%s", code, stderr)
	}
	// As serve mirrors them.
	st, err := store.Open(t.Context(), strings.TrimPrefix(env["NBPDNS_DATABASE_URL"], "sqlite://"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.MirrorFileGroups(t.Context(), []config.Group{{Name: "site-a", Views: []string{"v"}, DriftPolicy: "report"}},
		audit.File("/etc/nbpdns/nbpdns.yaml"))
	_ = st.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"groups", "set", "site-a", "--views", "w"},
		{"groups", "remove", "site-a"},
		{"groups", "rename", "site-a", "site-z"},
	} {
		if code, _, stderr := run(t, env, args...); code != exitError ||
			!strings.Contains(stderr, "server group site-a is managed by the config file, /etc/nbpdns/nbpdns.yaml; change it there") {
			t.Errorf("%s: exit %d:\n%s", strings.Join(args, " "), code, stderr)
		}
	}
	if code, stdout, _ := run(t, env, "groups", "list"); code != exitOK || !regexp.MustCompile(`(?m)^site-a +file +`).MatchString(stdout) {
		t.Errorf("groups list: exit %d:\n%s", code, stdout)
	}
}

// waitFor polls fn until it's true, or fails after 15 seconds.
func waitFor(t *testing.T, s *served, what string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !fn() {
		if time.Now().After(deadline) {
			t.Fatalf("%s didn't happen within 15s:\n%s", what, s.stderr)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// TestServeAppliesGroups adds, renames and removes a group while serve runs,
// and sees serve apply each within seconds; then restarts serve, which
// loads the CLI's group, and stops it when the config file claims its name.
func TestServeAppliesGroups(t *testing.T) {
	env, apiKey := groupsEnv(t)
	serveEnv := with(configFile(t, twoGroups), "NBPDNS_DATABASE_URL", env["NBPDNS_DATABASE_URL"],
		"NBPDNS_DATABASE_MASTER_KEY_FILE", env["NBPDNS_DATABASE_MASTER_KEY_FILE"],
		"NBPDNS_NETBOX_URL", "http://127.0.0.1:1", "NBPDNS_NETBOX_TOKEN", "nbt_abc.def")
	s := startServe(t, serveEnv)
	type status struct {
		Schedule struct {
			Refreshes struct{ Failed int } `json:"refreshes"`
		} `json:"schedule"`
	}
	refreshes := func() int {
		_, page := s.get("/status?json=1")
		var st status
		if err := json.Unmarshal([]byte(page), &st); err != nil {
			t.Fatal(err)
		}
		return st.Schedule.Refreshes.Failed
	}
	groups := func() string {
		code, page := s.api("/api/server-groups")
		var p struct {
			Items []struct {
				Name      string `json:"name"`
				ManagedBy string `json:"managed_by"`
			} `json:"items"`
		}
		if err := json.Unmarshal([]byte(page), &p); err != nil || code != 200 {
			t.Fatalf("%d %v: %s", code, err, page)
		}
		var out []string
		for _, g := range p.Items {
			out = append(out, g.Name+"/"+g.ManagedBy)
		}
		return strings.Join(out, ",")
	}
	waitFor(t, s, "the first refresh", func() bool { return refreshes() >= 1 })
	if got := groups(); got != "site-a/file,site-b/file" {
		t.Errorf("groups at start = %s", got)
	}

	if code, _, stderr := run(t, env, "groups", "add", "site-c", "--url", "http://127.0.0.1:1", "--views", "_default_",
		"--api-key-file", apiKey, "--no-check"); code != exitOK {
		t.Fatalf("groups add: exit %d:\n%s", code, stderr)
	}
	waitFor(t, s, "site-c's addition", func() bool { return groups() == "site-a/file,site-b/file,site-c/cli" })
	// A new group is compared at once, not a drift.interval later.
	waitFor(t, s, "a refresh for site-c", func() bool { return refreshes() >= 2 })

	if code, _, stderr := run(t, env, "groups", "rename", "site-c", "site-z"); code != exitOK {
		t.Fatalf("groups rename: exit %d:\n%s", code, stderr)
	}
	waitFor(t, s, "site-c's rename", func() bool { return groups() == "site-a/file,site-b/file,site-z/cli" })
	if code, _, stderr := run(t, env, "groups", "remove", "site-z"); code != exitOK {
		t.Fatalf("groups remove: exit %d:\n%s", code, stderr)
	}
	waitFor(t, s, "site-z's removal", func() bool { return groups() == "site-a/file,site-b/file" })
	if _, page := s.get("/metrics"); strings.Contains(page, `group="site-c"`) || strings.Contains(page, `group="site-z"`) {
		t.Errorf("the metrics keep a removed group's series:\n%s", page)
	}
	for _, want := range []string{`"msg":"applied the changed server groups","added":["site-c"]`, `"renamed":["site-c->site-z"]`,
		`"removed":["site-z"]`} {
		if !strings.Contains(s.stderr.String(), want) {
			t.Errorf("no %s in serve's log:\n%s", want, s.stderr)
		}
	}

	// After a restart, the CLI's group is compared from the start.
	if code, _, stderr := run(t, env, "groups", "add", "site-y", "--url", "http://127.0.0.1:1", "--views", "_default_",
		"--api-key-file", apiKey, "--no-check"); code != exitOK {
		t.Fatalf("groups add: exit %d:\n%s", code, stderr)
	}
	if code := s.stop(); code != exitOK {
		t.Fatalf("exit %d:\n%s", code, s.stderr)
	}
	s = startServe(t, serveEnv)
	if got := groups(); got != "site-a/file,site-b/file,site-y/cli" {
		t.Errorf("groups after a restart = %s", got)
	}
	if code := s.stop(); code != exitOK {
		t.Fatalf("exit %d:\n%s", code, s.stderr)
	}

	// A config file that claims the CLI's group's name stops serve.
	claimed := with(configFile(t, twoGroups+`    - name: site-y
      views: [_default_]
      primary: {url: "http://127.0.0.1:1", api_key: k}
`), "NBPDNS_DATABASE_URL", env["NBPDNS_DATABASE_URL"], "NBPDNS_DATABASE_MASTER_KEY_FILE", env["NBPDNS_DATABASE_MASTER_KEY_FILE"],
		"NBPDNS_NETBOX_URL", "http://127.0.0.1:1", "NBPDNS_NETBOX_TOKEN", "nbt_abc.def", "NBPDNS_SERVER_LISTEN", "127.0.0.1:0")
	if code, _, stderr := run(t, claimed, "serve"); code != exitError ||
		!strings.Contains(stderr, "server group site-y is in the config file, and os-user:") {
		t.Errorf("serve with a conflict: exit %d:\n%s", code, stderr)
	}
	// Without the master key, the CLI's group's key doesn't open.
	noKey := with(serveEnv, "NBPDNS_DATABASE_MASTER_KEY_FILE", "")
	if code, _, stderr := run(t, noKey, "serve"); code != exitError ||
		!strings.Contains(stderr, "the database stores server group site-y's API key, which doesn't open") {
		t.Errorf("serve without the master key: exit %d:\n%s", code, stderr)
	}
}

// TestUpdateClients checks that a change of the groups makes clients for
// new and changed groups, and keeps the rest.
func TestUpdateClients(t *testing.T) {
	s := testSession(t)
	g := func(name, url string) config.Group {
		return config.Group{Name: name, Primary: config.Primary{URL: url, APIKey: config.NewSecret("k"), ServerID: "localhost"}}
	}
	s.cfg.NetBox = config.NetBoxConfig{URL: "http://127.0.0.1:1", Token: config.NewSecret("nbt_a.b"), Timeout: time.Second, PageSize: 10, Concurrency: 1}
	s.cfg.PowerDNS.Groups = []config.Group{g("site-a", "http://127.0.0.1:1"), g("site-b", "http://127.0.0.1:1")}
	c, err := s.newClients(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer c.close()
	a := c.groups[0].c

	next := *s.cfg
	next.PowerDNS.Groups = []config.Group{g("site-a", "http://127.0.0.1:1"), g("site-c", "http://127.0.0.1:2")}
	c.reconfigure(t.Context(), &next)
	if len(c.groups) != 2 || c.groups[0].c != a || c.groups[1].cfg.Name != "site-c" || c.groups[1].c == nil {
		t.Errorf("clients after site-b gave way to site-c = %+v", c.groups)
	}
	moved := next
	moved.PowerDNS.Groups = []config.Group{g("site-a", "http://127.0.0.1:3")}
	c.reconfigure(t.Context(), &moved)
	if len(c.groups) != 1 || c.groups[0].c == a || c.groups[0].cfg.Primary.URL != "http://127.0.0.1:3" {
		t.Errorf("clients after site-a's URL changed = %+v", c.groups)
	}
}
