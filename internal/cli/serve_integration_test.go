//go:build integration

package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite" // the database/sql driver "sqlite", to tamper with the trail

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/lab"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/service"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
)

// labGroup returns a config file's server group named name, whose primary
// is at url, which serves f's view and ignores its ignored zone.
func labGroup(f *lab.DriftFixture, name, url string) string {
	return fmt.Sprintf("    - name: %s\n      views: [%s]\n      zone_policies: {%s: ignore}\n      primary: {url: %q, api_key: %q}\n",
		name, f.View, strings.TrimSuffix(f.Ignored, "."), url, lab.PowerDNSAPIKey)
}

// TestServe runs `nbpdns serve` on the drift fixture, in the lab's NetBox
// and on lab-a's primary, with a second group whose primary refuses
// connections, and reads its metrics.
func TestServe(t *testing.T) {
	nb, p := lab.NetBoxes[0], lab.PowerDNSes[0]
	f := lab.NewDriftFixture(t, nb, p)
	env := configFile(t, "powerdns:\n  groups:\n"+labGroup(f, p.Group, p.URL())+labGroup(f, "down", "http://127.0.0.1:1"))
	env["NBPDNS_NETBOX_URL"], env["NBPDNS_NETBOX_TOKEN"] = nb.URL(), f.ReaderToken
	env["NBPDNS_DRIFT_INTERVAL"] = "10s"

	s := startServe(t, env)
	s.waitReady(2 * time.Minute)
	code, body := s.get("/metrics")
	if code != http.StatusOK {
		t.Fatalf("metrics: %d", code)
	}
	for _, want := range []string{
		`nbpdns_drift_zones{group="lab-a",state="in_sync"} 1`,
		`nbpdns_drift_zones{group="lab-a",state="drift"} 1`,
		`nbpdns_drift_zones{group="lab-a",state="missing"} 1`,
		`nbpdns_drift_zones{group="lab-a",state="inactive_in_netbox"} 1`,
		`nbpdns_drift_zones{group="lab-a",state="ignored"} 1`,
		fmt.Sprintf(`nbpdns_drift_zone_drifted{group="lab-a",state="drift",zone=%q} 1`, f.Drift),
		fmt.Sprintf(`nbpdns_drift_zone_drifted{group="lab-a",state="missing",zone=%q} 1`, f.Missing),
		fmt.Sprintf(`nbpdns_drift_zone_drifted{group="lab-a",state="inactive_in_netbox",zone=%q} 1`, f.Parked),
		// www and mail, by value and TTL, and the SOA, by its contact.
		`nbpdns_drift_rrset_changes{group="lab-a",kind="changed"} 3`,
		`nbpdns_drift_rrset_changes{group="lab-a",kind="missing"} 1`,
		`nbpdns_drift_rrset_changes{group="lab-a",kind="extra"} 1`,
		`nbpdns_server_group_up{group="lab-a"} 1`,
		`nbpdns_server_group_up{group="down"} 0`,
		"nbpdns_netbox_up 1",
		`nbpdns_drift_refreshes_total{outcome="incomplete"} 1`,
		`nbpdns_http_client_requests_total{code="200",method="GET",service="NetBox",target="netbox"}`,
		`nbpdns_http_client_requests_total{code="200",method="GET",service="PowerDNS",target="lab-a"}`,
		`nbpdns_http_client_requests_total{code="error",method="GET",service="PowerDNS",target="down"}`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics have no %q", want)
		}
	}
	if t.Failed() {
		t.Logf("metrics:\n%s", body)
	}
	// The zone in sync has no drifted series.
	if strings.Contains(body, fmt.Sprintf("zone=%q", f.InSync)) {
		t.Errorf("the zone in sync has a drifted series")
	}
	// The status page lists the same drifted zones, in both forms.
	code, page := s.get("/status?json=1")
	var st service.Status
	if err := json.Unmarshal([]byte(page), &st); err != nil || code != http.StatusOK || len(st.Groups) != 2 {
		t.Fatalf("status: %d, %v:\n%s", code, err, page)
	}
	a, down := st.Groups[0], st.Groups[1]
	got := map[string]string{}
	for _, z := range a.DriftedZones {
		got[z.Zone] = z.State
	}
	if a.Name != "lab-a" || a.Status != "ok" || !reflect.DeepEqual(got, map[string]string{f.Drift: "drift", f.Missing: "missing", f.Parked: "inactive_in_netbox"}) {
		t.Errorf("lab-a's status: %+v", a)
	}
	if down.Name != "down" || down.Status != "failed" || !strings.Contains(down.Error, "isn't reachable") {
		t.Errorf("down's status: %+v", down)
	}
	if _, text := s.get("/status"); !strings.Contains(text, f.Drift) || !strings.Contains(text, f.Parked) {
		t.Errorf("the status page lacks the drifted zones:\n%s", text)
	}
	// The API's status, checked against its OpenAPI document.
	if code, page := s.api("/api/status"); code != http.StatusOK || !strings.Contains(page, `"up":true`) ||
		!strings.Contains(page, `"outcome":"incomplete"`) {
		t.Errorf("/api/status: %d:\n%s", code, page)
	}
	// The API's groups: lab-a compared, down failed.
	code, page = s.api("/api/server-groups")
	var groups struct {
		Items []struct {
			Name, Status string
			Counts       *drift.Counts
		}
	}
	if err := json.Unmarshal([]byte(page), &groups); err != nil || code != http.StatusOK || len(groups.Items) != 2 {
		t.Fatalf("/api/server-groups: %d, %v:\n%s", code, err, page)
	}
	if g := groups.Items[0]; g.Name != "lab-a" || g.Status != "ok" || g.Counts == nil || g.Counts.Drift != 1 || g.Counts.Missing != 1 {
		t.Errorf("lab-a in the API: %+v", g)
	}
	if g := groups.Items[1]; g.Name != "down" || g.Status != "failed" || g.Counts != nil {
		t.Errorf("down in the API: %+v", g)
	}
	if code, _ := s.api("/api/server-groups/down"); code != http.StatusOK {
		t.Errorf("/api/server-groups/down: %d", code)
	}
	// lab-a's drifted zones, and the drifted zone's changes: www and mail
	// changed, and the SOA, by its contact, one missing and one extra.
	code, page = s.api("/api/server-groups/lab-a/zones?state=drift,missing,inactive_in_netbox")
	var zones struct {
		Items []struct{ Zone, State string }
	}
	if err := json.Unmarshal([]byte(page), &zones); err != nil || code != http.StatusOK {
		t.Fatalf("lab-a's zones: %d, %v:\n%s", code, err, page)
	}
	got = map[string]string{}
	for _, z := range zones.Items {
		got[z.Zone] = z.State
	}
	if !reflect.DeepEqual(got, map[string]string{f.Drift: "drift", f.Missing: "missing", f.Parked: "inactive_in_netbox"}) {
		t.Errorf("lab-a's drifted zones in the API: %v", got)
	}
	code, page = s.api("/api/server-groups/lab-a/zones/" + f.Drift + "/changes")
	var changes struct {
		Items []struct{ Name, Type, Kind string }
	}
	if err := json.Unmarshal([]byte(page), &changes); err != nil || code != http.StatusOK {
		t.Fatalf("the drifted zone's changes: %d, %v:\n%s", code, err, page)
	}
	kinds := map[string]int{}
	for _, c := range changes.Items {
		kinds[c.Kind]++
	}
	if !reflect.DeepEqual(kinds, map[string]int{"changed": 3, "missing": 1, "extra": 1}) {
		t.Errorf("the drifted zone's changes in the API: %v:\n%s", kinds, page)
	}
	if code, _ := s.api("/api/server-groups/lab-a/zones/" + strings.TrimSuffix(f.InSync, ".")); code != http.StatusOK {
		t.Errorf("the zone in sync, without its final dot: %d", code)
	}
	// NetBox's records of the zone in sync, as nbpdns netbox records shows
	// them, and of the missing zone, which the primary doesn't have.
	for _, zone := range []string{f.InSync, f.Missing} {
		code, page = s.api("/api/server-groups/lab-a/zones/" + zone + "/rrsets?limit=1000")
		var rrsets struct {
			Items []struct {
				Name, Type string
				TTL        uint32
				Records    []struct{ Value string }
			}
		}
		if err := json.Unmarshal([]byte(page), &rrsets); err != nil || code != http.StatusOK || len(rrsets.Items) == 0 {
			t.Fatalf("%s's rrsets: %d, %v:\n%s", zone, code, err, page)
		}
		_, out, stderr := run(t, map[string]string{"NBPDNS_NETBOX_URL": nb.URL(), "NBPDNS_NETBOX_TOKEN": f.ReaderToken},
			"netbox", "records", "--zone", zone, "--view", f.View, "-o", "json")
		var want struct {
			RRsets []struct {
				Name, Type string
				TTL        uint32
				Records    []struct {
					Value  string
					Active bool
				}
			}
		}
		if err := json.Unmarshal([]byte(out), &want); err != nil {
			t.Fatalf("nbpdns netbox records %s: %v:\n%s%s", zone, err, out, stderr)
		}
		var got, exp []string
		for _, r := range rrsets.Items {
			for _, rec := range r.Records {
				got = append(got, fmt.Sprintf("%s %s %d %s", r.Name, r.Type, r.TTL, rec.Value))
			}
		}
		for _, r := range want.RRsets {
			for _, rec := range r.Records {
				if rec.Active {
					exp = append(exp, fmt.Sprintf("%s %s %d %s", r.Name, r.Type, r.TTL, rec.Value))
				}
			}
		}
		if !reflect.DeepEqual(got, exp) {
			t.Errorf("%s's rrsets in the API:\n%s\nnbpdns netbox records:\n%s", zone, strings.Join(got, "\n"), strings.Join(exp, "\n"))
		}
	}
	if code := s.stop(); code != exitOK {
		t.Errorf("exit %d, want 0:\n%s", code, s.stderr)
	}
	if !strings.Contains(s.stderr.String(), "a server group couldn't be read, so it keeps its last report") {
		t.Errorf("the failed group wasn't logged:\n%s", s.stderr)
	}
}

// TestServeRestarts restarts serve on the lab, and sees it serve its last
// reports and history at once, while a second serve on its database
// refuses to start. Then nbpdns audit verify passes on the trail that the
// two runs wrote, and fails once one of its events is edited.
func TestServeRestarts(t *testing.T) {
	nb, p := lab.NetBoxes[0], lab.PowerDNSes[0]
	f := lab.NewDriftFixture(t, nb, p)
	db := filepath.Join(t.TempDir(), "nbpdns.db")
	env := with(configFile(t, "powerdns:\n  groups:\n"+labGroup(f, p.Group, p.URL())),
		"NBPDNS_NETBOX_URL", nb.URL(), "NBPDNS_NETBOX_TOKEN", f.ReaderToken, "NBPDNS_DATABASE_URL", "sqlite://"+db)
	// reports returns lab-a's drifted zones, and the drifted zone's history,
	// as the API serves them.
	reports := func(s *served) (map[string]string, []map[string]any) {
		t.Helper()
		code, page := s.api("/api/server-groups/lab-a/zones?state=drift,missing,inactive_in_netbox")
		var zones struct {
			Items []struct{ Zone, State string }
		}
		if err := json.Unmarshal([]byte(page), &zones); err != nil || code != http.StatusOK {
			t.Fatalf("lab-a's zones: %d, %v:\n%s", code, err, page)
		}
		states := map[string]string{}
		for _, z := range zones.Items {
			states[z.Zone] = z.State
		}
		code, page = s.api("/api/server-groups/lab-a/zones/" + f.Drift + "/history")
		var history struct{ Items []map[string]any }
		if err := json.Unmarshal([]byte(page), &history); err != nil || code != http.StatusOK {
			t.Fatalf("the drifted zone's history: %d, %v:\n%s", code, err, page)
		}
		return states, history.Items
	}

	s := startServe(t, env)
	s.waitReady(2 * time.Minute)
	states, history := reports(s)
	if want := map[string]string{f.Drift: "drift", f.Missing: "missing", f.Parked: "inactive_in_netbox"}; !reflect.DeepEqual(states, want) {
		t.Errorf("lab-a's drifted zones = %v, want %v", states, want)
	}
	if len(history) != 1 || history[0]["state"] != "drift" {
		t.Fatalf("the drifted zone's history = %v, want one drift row", history)
	}
	_, page := s.get("/status?json=1")
	var st service.Status
	if err := json.Unmarshal([]byte(page), &st); err != nil || st.Schedule.LastRefresh == nil {
		t.Fatalf("status: %v:\n%s", err, page)
	}
	last := st.Schedule.LastRefresh.Finished

	if code, _, stderr := run(t, env, "serve"); code != exitError || !strings.Contains(stderr, "another nbpdns holds the database's lock") {
		t.Errorf("a second serve: exit %d, want %d:\n%s", code, exitError, stderr)
	}
	if code := s.stop(); code != exitOK {
		t.Fatalf("exit %d:\n%s", code, s.stderr)
	}

	// With NetBox unreachable, a restarted serve can only serve what it
	// kept, and it's ready with it before any refresh.
	s = startServe(t, with(env, "NBPDNS_NETBOX_URL", "http://127.0.0.1:1"))
	if code, _ := s.get("/readyz"); code != http.StatusOK {
		t.Errorf("readyz after a restart: %d, want 200 at once", code)
	}
	code, page := s.api("/api/status")
	var status struct {
		Database struct {
			Restored     bool       `json:"restored"`
			RestoredAsOf *time.Time `json:"restored_as_of"`
		} `json:"database"`
	}
	if err := json.Unmarshal([]byte(page), &status); err != nil || code != http.StatusOK {
		t.Fatalf("/api/status: %d, %v:\n%s", code, err, page)
	}
	if d := status.Database; !d.Restored || d.RestoredAsOf == nil || d.RestoredAsOf.Sub(last).Abs() > time.Second {
		t.Errorf("/api/status's database = %s, want the reports restored as of %s", page, last)
	}
	if got, h := reports(s); !reflect.DeepEqual(got, states) || !reflect.DeepEqual(h, history) {
		t.Errorf("after a restart, lab-a's drifted zones = %v, and the history = %v; want %v and %v", got, h, states, history)
	}
	dbEnv := map[string]string{"NBPDNS_DATABASE_URL": env["NBPDNS_DATABASE_URL"]}
	code, stdout, stderr := run(t, dbEnv, "drift", "history", "--group", "lab-a", "--zone", f.Drift, "-o", "json")
	var rows []struct {
		RefreshID float64 `json:"refresh_id"`
		State     string  `json:"state"`
	}
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil || code != exitOK || len(rows) != 1 ||
		rows[0].State != "drift" || rows[0].RefreshID != history[0]["refresh_id"] {
		t.Errorf("drift history: exit %d, %v, want the API's row, %v:\n%s%s", code, err, history[0], stdout, stderr)
	}
	if code := s.stop(); code != exitOK {
		t.Fatalf("exit %d:\n%s", code, s.stderr)
	}

	code, stdout, stderr = run(t, dbEnv, "audit", "verify")
	if code != exitOK || !strings.HasPrefix(stdout, "The audit trail is intact: ") {
		t.Fatalf("audit verify: exit %d:\n%s%s", code, stdout, stderr)
	}
	code, stdout, stderr = run(t, dbEnv, "audit", "list", "-o", "json", "--target", "zone:lab-a/"+f.Drift)
	var events []struct {
		Seq    int64          `json:"seq"`
		Action string         `json:"action"`
		After  map[string]any `json:"after"`
	}
	if err := json.Unmarshal([]byte(stdout), &events); err != nil || code != exitOK || len(events) != 1 ||
		events[0].Action != string(audit.ZoneDriftChanged) || events[0].After["state"] != "drift" {
		t.Fatalf("the drifted zone's events: exit %d, %v, want one %s to drift:\n%s%s", code, err, audit.ZoneDriftChanged, stdout, stderr)
	}
	// An event edited to say the zone was in sync breaks the chain there.
	raw, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.ExecContext(t.Context(), `UPDATE audit_events SET after_json = '{"changes":0,"policy":"report","state":"in_sync"}' WHERE seq = ?`,
		events[0].Seq); err != nil {
		t.Fatal(err)
	}
	code, _, stderr = run(t, dbEnv, "audit", "verify")
	if want := fmt.Sprintf("the audit trail is broken at event %d: its contents don't match its hash", events[0].Seq); code != exitError ||
		!strings.Contains(stderr, want) {
		t.Errorf("audit verify of an edited trail: exit %d, want %d with %q:\n%s", code, exitError, want, stderr)
	}
}

// TestServeAppliesCLIChanges sets drift.interval, and adds a server group
// whose check reads the lab's PowerDNS, with the command line while serve
// runs on the lab, and sees serve apply both within seconds.
func TestServeAppliesCLIChanges(t *testing.T) {
	nb, p := lab.NetBoxes[0], lab.PowerDNSes[0]
	f := lab.NewDriftFixture(t, nb, p)
	_, keyPath := keyFile(t)
	env := with(configFile(t, "powerdns:\n  groups:\n"+labGroup(f, p.Group, p.URL())),
		"NBPDNS_NETBOX_URL", nb.URL(), "NBPDNS_NETBOX_TOKEN", f.ReaderToken,
		"NBPDNS_DATABASE_URL", "sqlite://"+filepath.Join(t.TempDir(), "nbpdns.db"), "NBPDNS_DATABASE_MASTER_KEY_FILE", keyPath)
	s := startServe(t, env)
	s.waitReady(2 * time.Minute)

	// The next refresh moves from drift.interval's default, 5m, after the
	// last one started, to 15s.
	if code, _, stderr := run(t, env, "settings", "set", "drift.interval", "15s", "--reason", "a test"); code != exitOK {
		t.Fatalf("settings set: exit %d:\n%s", code, stderr)
	}
	waitFor(t, s, "the new interval", func() bool {
		_, page := s.get("/status?json=1")
		var st service.Status
		if err := json.Unmarshal([]byte(page), &st); err != nil {
			t.Fatalf("status: %v:\n%s", err, page)
		}
		sc := st.Schedule
		return sc.IntervalSeconds == 15 && sc.LastRefresh != nil && sc.NextRefresh != nil &&
			!sc.NextRefresh.After(sc.LastRefresh.Started.Add(15*time.Second))
	})

	// A group on lab-a's primary and view, with its check, is compared at
	// once, and finds what lab-a finds.
	apiKey := filepath.Join(t.TempDir(), "api-key")
	if err := os.WriteFile(apiKey, []byte(lab.PowerDNSAPIKey+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := run(t, env, "groups", "add", "lab-cli", "--url", p.URL(), "--views", f.View,
		"--zone-policies", strings.TrimSuffix(f.Ignored, ".")+"=ignore", "--api-key-file", apiKey, "--reason", "a test"); code != exitOK {
		t.Fatalf("groups add: exit %d:\n%s", code, stderr)
	}
	type group struct {
		Name      string        `json:"name"`
		Status    string        `json:"status"`
		ManagedBy string        `json:"managed_by"`
		Counts    *drift.Counts `json:"counts"`
	}
	groups := func() map[string]group {
		code, page := s.api("/api/server-groups")
		var list struct{ Items []group }
		if err := json.Unmarshal([]byte(page), &list); err != nil || code != http.StatusOK {
			t.Fatalf("/api/server-groups: %d, %v:\n%s", code, err, page)
		}
		byName := map[string]group{}
		for _, g := range list.Items {
			byName[g.Name] = g
		}
		return byName
	}
	waitFor(t, s, "lab-cli's comparison", func() bool { return groups()["lab-cli"].Status == "ok" })
	g := groups()
	if a, c := g["lab-a"], g["lab-cli"]; c.ManagedBy != "cli" || a.Counts == nil || c.Counts == nil || *c.Counts != *a.Counts {
		t.Errorf("lab-cli is managed by %q, with %+v; want cli, with lab-a's %+v", c.ManagedBy, c.Counts, a.Counts)
	}

	code, stdout, stderr := run(t, env, "audit", "list", "-o", "json", "--target", "server-group:lab-cli")
	var events []struct {
		Actor, Action, Reason string
		After                 map[string]any
	}
	if err := json.Unmarshal([]byte(stdout), &events); err != nil || code != exitOK || len(events) != 1 {
		t.Fatalf("lab-cli's events: exit %d, %v:\n%s%s", code, err, stdout, stderr)
	}
	if e := events[0]; e.Action != string(audit.GroupCreated) || e.Actor != audit.OSUser().String() || e.Reason != "a test" ||
		e.After["check"] != store.CheckPassed {
		t.Errorf("lab-cli's event = %+v, want its creation by %s, with its check passed", e, audit.OSUser())
	}
	if code := s.stop(); code != exitOK {
		t.Errorf("exit %d:\n%s", code, s.stderr)
	}
}
