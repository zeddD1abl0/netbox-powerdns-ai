//go:build integration

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/lab"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/metrics"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/secrets"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/service"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/version"
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

// TestServeOnPostgres runs two replicas of serve on PostgreSQL, and checks
// that /status says so without the password (ADR-0046); that one of them
// leads and refreshes, and the other follows; that db migrate refuses while
// either runs; and that the follower takes over when the leader stops, which
// the audit trail records (ADR-0047).
func TestServeOnPostgres(t *testing.T) {
	where := lab.NewPostgresDatabase(t)
	shown := strings.Replace(where, lab.PostgresPassword, "xxxxx", 1)
	env := configFile(t, twoGroups)
	env["NBPDNS_NETBOX_URL"], env["NBPDNS_NETBOX_TOKEN"] = "http://127.0.0.1:1", "nbt_abc.def"
	env["NBPDNS_DATABASE_URL"] = where
	env["NBPDNS_DATABASE_POLL_INTERVAL"] = "100ms"
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

	// The first serve leads, and the second follows it.
	leader := first.statusWhen(t, func(st service.Status) bool { return st.Replica.Role == service.RoleLeader })
	second := startServe(t, env)
	follower := second.statusWhen(t, func(st service.Status) bool { return st.Leader != nil })
	if follower.Replica.Role != service.RoleFollower || follower.Leader.ID != leader.Replica.ID ||
		follower.Replica.ID == leader.Replica.ID || follower.Replica.Name != leader.Replica.Name {
		t.Errorf("the second serve: replica %+v, leader %+v, want a follower of %+v", follower.Replica, follower.Leader, leader.Replica)
	}
	// The follower runs no refresh, and serves the leader's (ADR-0048).
	if follower.Schedule.Refreshes != (service.Refreshes{}) {
		t.Errorf("the follower refreshed: %+v", follower.Schedule)
	}
	if lr := second.statusWhen(t, func(st service.Status) bool { return st.Schedule.LastRefresh != nil }).Schedule.LastRefresh; lr.Outcome != "failed" ||
		!lr.Finished.Equal(first.statusWhen(t, func(service.Status) bool { return true }).Schedule.LastRefresh.Finished) {
		t.Errorf("the follower's last refresh = %+v, want the leader's", lr)
	}
	for _, r := range []struct {
		s    *served
		want []string
	}{
		{first, []string{"nbpdns_leader 1", "nbpdns_leader_changes_total 0", `nbpdns_drift_refreshes_total{outcome="failed"} 1`}},
		{second, []string{"nbpdns_leader 0", "nbpdns_leader_changes_total 0", `nbpdns_drift_refreshes_total{outcome="failed"} 0`,
			"nbpdns_leader_since_timestamp_seconds "}},
	} {
		if _, body := r.s.get("/metrics"); !containsAll(body, r.want) {
			t.Errorf("the metrics of %s have no %q:\n%s", r.s.addr, r.want, body)
		}
	}
	if code, api := second.api("/api/status"); code != http.StatusOK || !strings.Contains(api, `"role":"follower"`) {
		t.Errorf("the follower's /api/status: %d:\n%s", code, api)
	}

	other := with(env, "NBPDNS_SERVER_LISTEN", "127.0.0.1:0")
	host, _ := os.Hostname()
	code, _, stderr := run(t, other, "db", "migrate")
	want := fmt.Sprintf("2 other nbpdns processes hold the database's lock: host %s, PID %d, since ", host, os.Getpid())
	if code != exitError || !strings.Contains(stderr, want) {
		t.Errorf("db migrate while two serves run: exit %d, want %d with %q:\n%s", code, exitError, want, stderr)
	}
	pages.WriteString(stderr)

	// The leader stops, and releases its lock: the follower takes over at
	// its next poll.
	if code := first.stop(); code != exitOK {
		t.Fatalf("exit %d:\n%s", code, first.stderr)
	}
	took := second.statusWhen(t, func(st service.Status) bool {
		return st.Replica.Role == service.RoleLeader && st.Schedule.LastRefresh != nil
	})
	if took.Leader == nil || took.Leader.ID != follower.Replica.ID {
		t.Errorf("after the takeover: leader %+v, want the second serve", took.Leader)
	}
	if _, body := second.get("/metrics"); !containsAll(body, []string{"nbpdns_leader 1", "nbpdns_leader_changes_total 1",
		`nbpdns_audit_events_total{action="service.leader_acquired"} 1`}) {
		t.Errorf("the new leader's metrics:\n%s", body)
	}
	_, stdout, _ := run(t, other, "audit", "list", "--action", "service.leader_acquired", "-o", "json")
	var events []auditEvent
	if err := json.Unmarshal([]byte(stdout), &events); err != nil || len(events) != 2 {
		t.Fatalf("audit list: %v, want two takeovers:\n%s", err, stdout)
	}
	var before struct {
		Replica    string    `json:"replica"`
		InstanceID string    `json:"instance_id"`
		LastSeen   time.Time `json:"last_seen"`
	}
	if err := json.Unmarshal(events[1].Before, &before); err != nil || before.Replica != host || before.InstanceID != leader.Replica.ID ||
		before.LastSeen.IsZero() || string(events[0].Before) != "null" || events[1].Target != "service:"+host {
		t.Errorf("the takeovers: %+v, %+v, the second's before %s", events[0], events[1], events[1].Before)
	}

	// A serve that starts again follows the new leader.
	again := startServe(t, env)
	if got := loggedDatabaseID(t, again.stderr.String()); got != id {
		t.Errorf("after a restart, the database's ID is %s, want %s", got, id)
	}
	if st := again.statusWhen(t, func(st service.Status) bool { return st.Leader != nil }); st.Leader.ID != took.Replica.ID {
		t.Errorf("the restarted serve follows %+v, want the second serve", st.Leader)
	}
	for name, out := range map[string]string{"/status and db migrate": pages.String(),
		"the first serve's log": first.stderr.String(), "the second serve's log": second.stderr.String(),
		"the restarted serve's log": again.stderr.String()} {
		if strings.Contains(out, lab.PostgresPassword) {
			t.Errorf("%s shows the database's password:\n%s", name, out)
		}
	}
	if !strings.Contains(first.stderr.String(), "the connection to PostgreSQL isn't encrypted") {
		t.Errorf("serve didn't warn of the unencrypted connection:\n%s", first.stderr)
	}
}

// TestReplicasServeTheSameAnswers runs two replicas on PostgreSQL, with the
// lab's NetBox and PowerDNS, and checks that the follower, once it has
// polled, serves what the leader does: the API's answers, NetBox's records
// included, the deployment's status, and the drift gauges (ADR-0048). A
// follower that starts once the leader has refreshed is ready at once, and
// a refresh after that reaches it at its next poll.
func TestReplicasServeTheSameAnswers(t *testing.T) {
	nb, p := lab.NetBoxes[0], lab.PowerDNSes[0]
	f := lab.NewDriftFixture(t, nb, p)
	_, keyPath := keyFile(t)
	env := with(configFile(t, "powerdns:\n  groups:\n"+labGroup(f, p.Group, p.URL())),
		"NBPDNS_NETBOX_URL", nb.URL(), "NBPDNS_NETBOX_TOKEN", f.ReaderToken, "NBPDNS_DATABASE_URL", lab.NewPostgresDatabase(t),
		"NBPDNS_DATABASE_POLL_INTERVAL", "100ms", "NBPDNS_DATABASE_MASTER_KEY_FILE", keyPath)
	leader := startServe(t, env)
	leader.waitReady(2 * time.Minute)
	follower := startServe(t, env)
	if code, body := follower.get("/readyz"); code != http.StatusOK {
		t.Errorf("a follower that starts after the leader's refresh: /readyz = %d %q, want 200 at once", code, body)
	}

	paths := []string{"/api/server-groups"}
	of := func(group string) {
		paths = append(paths, "/api/server-groups/"+group+"/zones", "/api/server-groups/"+group+"/refreshes")
		for _, z := range []string{f.Drift, f.Missing, f.Parked} {
			zone := "/api/server-groups/" + group + "/zones/" + z
			paths = append(paths, zone, zone+"/changes", zone+"/rrsets", zone+"/history")
		}
	}
	of("lab-a")
	// answers returns what s serves of the deployment: each path's answer,
	// with its links on one host, the deployment's facts in its status, and
	// its drift gauges.
	answers := func(s *served) string {
		var b strings.Builder
		for _, path := range paths {
			code, body := s.get(path)
			fmt.Fprintf(&b, "%s: %d %s\n", path, code, strings.ReplaceAll(body, s.addr, "nbpdns"))
		}
		_, page := s.get("/status?json=1")
		var st service.Status
		if err := json.Unmarshal([]byte(page), &st); err != nil {
			t.Fatalf("/status?json=1: %v:\n%s", err, page)
		}
		facts, err := json.Marshal([]any{st.Schedule.LastRefresh, st.Schedule.LastCompleteRefresh, st.Schedule.NextRefresh, st.NetBox, st.Groups})
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&b, "status: %s\n", facts)
		_, metrics := s.get("/metrics")
		for line := range strings.Lines(metrics) {
			for _, gauge := range []string{"nbpdns_drift_zones{", "nbpdns_drift_zone_drifted{", "nbpdns_drift_rrset_changes{",
				"nbpdns_server_group_up{", "nbpdns_server_group_last_success_timestamp_seconds{", "nbpdns_netbox_up ",
				"nbpdns_drift_last_refresh_timestamp_seconds ", "nbpdns_drift_last_complete_refresh_timestamp_seconds "} {
				if strings.HasPrefix(line, gauge) {
					b.WriteString(line)
				}
			}
		}
		return b.String()
	}
	// same waits until the follower serves what the leader does. The
	// follower polls every 100ms, and the leader's row has its next refresh
	// within a heartbeat, 5 seconds.
	same := func(step, group string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			want, got := answers(leader), answers(follower)
			if got == want {
				if !strings.Contains(want, "/"+group+"/zones/"+f.Drift+"/rrsets: 200") ||
					!strings.Contains(want, `nbpdns_drift_zone_drifted{group="`+group+`"`) {
					t.Errorf("%s: the leader serves no records, or no drift, of %s:\n%s", step, group, want)
				}
				// The follower's answers are the API's, as its spec has them.
				for _, path := range paths {
					follower.api(path)
				}
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: the follower serves\n%s\nwant the leader's\n%s", step, got, want)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	same("the leader's first refresh", "lab-a")

	// A group added with the command line has the leader refresh at once,
	// which the follower then serves too.
	apiKey := filepath.Join(t.TempDir(), "api-key")
	if err := os.WriteFile(apiKey, []byte(lab.PowerDNSAPIKey+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := run(t, env, "groups", "add", "lab-cli", "--url", p.URL(), "--views", f.View,
		"--zone-policies", strings.TrimSuffix(f.Ignored, ".")+"=ignore", "--api-key-file", apiKey, "--reason", "a test"); code != exitOK {
		t.Fatalf("groups add: exit %d:\n%s", code, stderr)
	}
	leader.statusWhen(t, func(st service.Status) bool {
		return len(st.Groups) == 2 && st.Groups[1].Name == "lab-cli" && st.Groups[1].Status == "ok"
	})
	of("lab-cli")
	same("the refresh of the group added", "lab-cli")
	if st := follower.statusWhen(t, func(service.Status) bool { return true }); st.Replica.Role != service.RoleFollower ||
		st.Schedule.Refreshes != (service.Refreshes{}) {
		t.Errorf("the follower: replica %+v, refreshes %+v, want a follower that ran none", st.Replica, st.Schedule.Refreshes)
	}
}

// TestLeaderStepsDown checks that a leader whose heartbeat misses its
// deadline steps down within 10 seconds, as ADR-0047 has it: it stops its
// work, and releases the leader's lock, which another replica then takes,
// and its leader-only writes fail.
func TestLeaderStepsDown(t *testing.T) {
	where := lab.NewPostgresDatabase(t)
	s := testSession(t)
	logs := &lockedBuffer{}
	s.log, s.metrics = slog.New(slog.NewJSONHandler(logs, nil)), metrics.New(version.Get())
	open := func() *store.Store {
		st, err := store.OpenPostgres(t.Context(), where)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = st.Close() })
		return st
	}
	st, other := open(), open()
	if _, err := st.Migrate(t.Context(), "test"); err != nil {
		t.Fatal(err)
	}
	me := store.Replica{Name: "host-a", ID: "A"}
	ss, err := st.Join(t.Context(), me)
	if err != nil {
		t.Fatal(err)
	}
	svc := service.New(service.Options{Replica: me, Elected: true, Log: s.log, Tracer: s.tracer, Metrics: s.metrics,
		Interval: time.Hour, Timeout: time.Minute})
	led, stopped := make(chan struct{}), make(chan time.Time, 1)
	// One check, at which the replica leads: it doesn't try again once it
	// steps down, so that another replica can take the lock.
	polls := make(chan poll, 1)
	polls <- poll{read: true}
	e := &election{s: s, st: st, svc: svc, me: me, ss: ss, polls: polls, lead: func(ctx context.Context) {
		close(led)
		<-ctx.Done()
		stopped <- time.Now()
	}}
	ctx, cancel := context.WithCancel(t.Context())
	ran := make(chan struct{})
	go func() {
		defer close(ran)
		e.run(ctx)
	}()
	defer func() {
		cancel()
		<-ran
	}()
	select {
	case <-led:
	case <-time.After(10 * time.Second):
		t.Fatal("the only replica didn't lead")
	}

	// Another session holds the leader's row, so the next heartbeat waits,
	// past its deadline.
	db, err := sql.Open("pgx", where)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(t.Context(), "SELECT * FROM leader FOR UPDATE"); err != nil {
		t.Fatal(err)
	}
	held := time.Now()
	var down time.Time
	select {
	case down = <-stopped:
	case <-time.After(15 * time.Second):
		t.Fatal("the leader didn't step down")
	}
	if d := down.Sub(held); d > heartbeatInterval+heartbeatTimeout+time.Second {
		t.Errorf("the leader stepped down %s after its heartbeat stalled, want within %s", d, heartbeatInterval+heartbeatTimeout)
	}
	// It follows once its work has stopped, and its session has ended.
	for deadline := time.Now().Add(5 * time.Second); svc.Status().Replica.Role != service.RoleFollower; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the replica didn't follow after it stepped down")
		}
	}
	if _, err := st.RecordRefresh(t.Context(), store.RefreshRecord{Kind: store.RefreshFull, Cause: store.CauseSchedule,
		Started: time.Now(), Finished: time.Now(), Outcome: "complete"}); !errors.Is(err, store.ErrNotLeader) {
		t.Errorf("a refresh's write after the step-down: error = %v, want ErrNotLeader", err)
	}
	if !strings.Contains(logs.String(), "the leader missed its heartbeat, so this replica steps down") {
		t.Errorf("no step-down in the logs:\n%s", logs)
	}
	// The lock is free: another replica takes it, once the row is.
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	b, err := other.Join(t.Context(), store.Replica{Name: "host-b", ID: "B"})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if tb, err := b.TryLead(t.Context(), time.Time{}); err != nil || tb == nil || tb.Previous == nil || tb.Previous.ID != "A" {
		t.Errorf("another replica's TryLead() after the step-down = %+v, %v, want a takeover from A", tb, err)
	}
}

// statusWhen polls the service's /status until cond holds of it, for up to
// 15 seconds, and returns it.
func (s *served) statusWhen(t *testing.T, cond func(service.Status) bool) service.Status {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		_, body := s.get("/status?json=1")
		var st service.Status
		if err := json.Unmarshal([]byte(body), &st); err != nil {
			t.Fatalf("/status?json=1: %v:\n%s", err, body)
		}
		if cond(st) {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatalf("/status didn't come to the state wanted:\n%s\n%s", body, s.stderr)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// containsAll reports whether s contains each of subs.
func containsAll(s string, subs []string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
