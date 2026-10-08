//go:build integration

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/lab"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/service"
)

// TestServe runs `nbpdns serve` on the drift fixture, in the lab's NetBox
// and on lab-a's primary, with a second group whose primary refuses
// connections, and reads its metrics.
func TestServe(t *testing.T) {
	nb, p := lab.NetBoxes[0], lab.PowerDNSes[0]
	f := lab.NewDriftFixture(t, nb, p)
	group := func(name, url string) string {
		return fmt.Sprintf("    - name: %s\n      views: [%s]\n      zone_policies: {%s: ignore}\n      primary: {url: %q, api_key: %q}\n",
			name, f.View, strings.TrimSuffix(f.Ignored, "."), url, lab.PowerDNSAPIKey)
	}
	env := configFile(t, "powerdns:\n  groups:\n"+group(p.Group, p.URL())+group("down", "http://127.0.0.1:1"))
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
