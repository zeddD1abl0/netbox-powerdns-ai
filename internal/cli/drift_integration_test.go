//go:build integration

package cli

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/lab"
)

// TestDrift runs nbpdns drift on the drift fixture, in the lab's NetBox and
// on lab-a's primary.
func TestDrift(t *testing.T) {
	nb, p := lab.NetBoxes[0], lab.PowerDNSes[0]
	f := lab.NewDriftFixture(t, nb, p)
	group := func(name, url string) string {
		return fmt.Sprintf("    - name: %s\n      views: [%s]\n      zone_policies: {%s: ignore}\n      primary: {url: %q, api_key: %q}\n",
			name, f.View, strings.TrimSuffix(f.Ignored, "."), url, lab.PowerDNSAPIKey)
	}
	env := func(groups ...string) map[string]string {
		e := configFile(t, "powerdns:\n  groups:\n"+strings.Join(groups, ""))
		e["NBPDNS_NETBOX_URL"], e["NBPDNS_NETBOX_TOKEN"] = nb.URL(), f.ReaderToken
		return e
	}
	labA := env(group(p.Group, p.URL()))
	report := func(t *testing.T, e map[string]string, wantCode int, args ...string) drift.Report {
		t.Helper()
		code, out, stderr := run(t, e, append([]string{"drift", "-o", "json"}, args...)...)
		var r drift.Report
		if err := json.Unmarshal([]byte(out), &r); err != nil || code != wantCode {
			t.Fatalf("exit %d, want %d, %v: %s\n%s", code, wantCode, err, out, stderr)
		}
		return r
	}

	t.Run("JSON", func(t *testing.T) {
		r := report(t, labA, exitDrift)
		if !r.Complete || !r.Drift || len(r.Groups) != 1 {
			t.Fatalf("report complete %v, drift %v, %d groups", r.Complete, r.Drift, len(r.Groups))
		}
		g := r.Groups[0]
		if g.Group != p.Group || g.Status != drift.StatusOK || len(g.Problems) != 0 || len(g.Warnings) != 0 {
			t.Errorf("group %s %s, problems %v, warnings %v", g.Group, g.Status, g.Problems, g.Warnings)
		}
		// Other tests' zones on the primary are unmanaged too, so only the
		// fixture's is looked for.
		if !slices.Contains(g.Unmanaged, f.Unmanaged) {
			t.Errorf("unmanaged %v, want %s among them", g.Unmanaged, f.Unmanaged)
		}
		if c := g.Counts; c.InSync != 1 || c.Drift != 1 || c.Missing != 1 || c.Inactive != 1 || c.Ignored != 1 {
			t.Errorf("counts %+v", c)
		}

		zones := map[string]drift.ZoneReport{}
		for _, z := range g.Zones {
			zones[z.Zone] = z
		}
		for zone, want := range map[string]string{
			f.InSync: drift.StateInSync, f.Drift: drift.StateDrift, f.Missing: drift.StateMissing,
			f.Parked: drift.StateInactive, f.Ignored: drift.StateIgnored,
		} {
			if z := zones[zone]; z.State != want || z.View != f.View {
				t.Errorf("zone %s: state %q in view %q, want %q in %q", zone, z.State, z.View, want, f.View)
			}
		}
		// Each side gave the SOA its own serial, which isn't compared.
		if z := zones[f.InSync]; len(z.Changes) != 0 || z.NetBoxSerial == 0 || z.PowerDNSSerial == 0 || z.NetBoxSerial == z.PowerDNSSerial {
			t.Errorf("zone %s: serials %d and %d, changes %+v", z.Zone, z.NetBoxSerial, z.PowerDNSSerial, z.Changes)
		}
		if z := zones[f.Ignored]; z.Policy != "ignore" || len(z.Changes) != 0 {
			t.Errorf("zone %s: policy %q, changes %+v", z.Zone, z.Policy, z.Changes)
		}

		d := f.Drift
		changes := map[[2]string]drift.Change{}
		for _, c := range zones[d].Changes {
			changes[[2]string{c.Name, c.Type}] = c
		}
		side := func(ttl uint32, values ...string) *drift.Side { return &drift.Side{TTL: ttl, Values: values} }
		for _, want := range []drift.Change{
			{Name: "gone." + d, Type: "A", Kind: drift.ChangeMissing, NetBox: side(3600, "192.0.2.20")},
			{Name: "stray." + d, Type: "TXT", Kind: drift.ChangeExtra, PowerDNS: side(3600, `"stray"`)},
			{Name: "www." + d, Type: "A", Kind: drift.ChangeChanged, NetBox: side(3600, "192.0.2.10"), PowerDNS: side(3600, "192.0.2.99")},
			{Name: "mail." + d, Type: "A", Kind: drift.ChangeChanged, NetBox: side(300, "192.0.2.25"), PowerDNS: side(600, "192.0.2.25")},
		} {
			if got := changes[[2]string{want.Name, want.Type}]; !reflect.DeepEqual(got, want) {
				t.Errorf("change %+v, want %+v", got, want)
			}
		}
		soa := changes[[2]string{d, "SOA"}]
		if soa.Kind != drift.ChangeChanged || soa.NetBox == nil || soa.PowerDNS == nil ||
			!strings.Contains(soa.NetBox.Values[0], " hostmaster."+d+" ") || !strings.Contains(soa.PowerDNS.Values[0], " admin."+d+" ") {
			t.Errorf("SOA change %+v", soa)
		}
		if len(changes) != 5 {
			t.Errorf("%d changes, want 5: %+v", len(changes), zones[d].Changes)
		}
	})

	t.Run("table", func(t *testing.T) {
		code, out, stderr := run(t, labA, "drift")
		if code != exitDrift || !strings.Contains(stderr, "3 zones drifted") {
			t.Fatalf("exit %d:\n%s\n%s", code, out, stderr)
		}
		d := f.Drift
		norm := oneSpace(out)
		for _, row := range []string{
			fmt.Sprintf("lab-a %s missing gone.%s A 3600 192.0.2.20 -", d, d),
			fmt.Sprintf(`lab-a %s extra stray.%s TXT - 3600 "stray"`, d, d),
			fmt.Sprintf("lab-a %s changed www.%s A 3600 192.0.2.10 3600 192.0.2.99", d, d),
			fmt.Sprintf("lab-a %s changed mail.%s A 300 192.0.2.25 600 192.0.2.25", d, d),
			"lab-a " + f.Missing + " zone missing on the primary",
			"lab-a " + f.Parked + " zone served, but not active in NetBox",
			"Ignored zones, not compared:\nGROUP ZONE\nlab-a " + f.Ignored,
			"lab-a " + f.Unmanaged,
		} {
			if !strings.Contains(norm, row) {
				t.Errorf("table has no %q:\n%s", row, out)
			}
		}
		// A zone in sync is only counted.
		if strings.Contains(out, f.InSync) {
			t.Errorf("table lists %s:\n%s", f.InSync, out)
		}
	})

	t.Run("one zone", func(t *testing.T) {
		// Only the zone is read, on both sides, so the primary's other zones
		// aren't listed as unmanaged.
		for _, tt := range []struct {
			zone          string
			code          int
			states        []string
			wantUnmanaged []string
		}{
			{f.InSync, exitOK, []string{drift.StateInSync}, []string{}},
			{strings.ToUpper(strings.TrimSuffix(f.Drift, ".")), exitDrift, []string{drift.StateDrift}, []string{}},
			{f.Unmanaged, exitOK, nil, []string{f.Unmanaged}},
		} {
			t.Run(tt.zone, func(t *testing.T) {
				g := report(t, labA, tt.code, "--zone", tt.zone).Groups[0]
				var states []string
				for _, z := range g.Zones {
					states = append(states, z.State)
				}
				if !slices.Equal(states, tt.states) || !slices.Equal(g.Unmanaged, tt.wantUnmanaged) {
					t.Errorf("states %v, unmanaged %v; want %v, %v", states, g.Unmanaged, tt.states, tt.wantUnmanaged)
				}
			})
		}
		nowhere := "nowhere-" + f.ID + ".nbpdns.example"
		code, _, stderr := run(t, labA, "drift", "--zone", nowhere)
		if code != exitError || !strings.Contains(stderr, "zone "+nowhere+". isn't in any server group's NetBox views") {
			t.Errorf("exit %d:\n%s", code, stderr)
		}
	})

	t.Run("a group that can't be read", func(t *testing.T) {
		// Its URL refuses connections, in the job's container as on a laptop.
		e := env(group(p.Group, p.URL()), group("down", "http://127.0.0.1:1"))
		code, out, stderr := run(t, e, "drift", "-o", "json")
		var r drift.Report
		if err := json.Unmarshal([]byte(out), &r); err != nil || code != exitError ||
			!strings.Contains(stderr, "1 of 2 server groups couldn't be read, so the report is incomplete") {
			t.Fatalf("exit %d, %v: %s\n%s", code, err, out, stderr)
		}
		if len(r.Groups) != 2 || r.Complete {
			t.Fatalf("report complete %v, %d groups", r.Complete, len(r.Groups))
		}
		a, down := r.Groups[0], r.Groups[1]
		if a.Status != drift.StatusOK || a.Counts.Drift != 1 {
			t.Errorf("group %s: %s, counts %+v", a.Group, a.Status, a.Counts)
		}
		if down.Status != drift.StatusFailed || !strings.Contains(down.Error, "isn't reachable") {
			t.Errorf("group %s: %s, %q", down.Group, down.Status, down.Error)
		}
	})
}
