//go:build integration

package cli

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/lab"
)

// labGroups returns a config file declaring the lab's server groups, each
// serving views, with key as every primary's API key.
func labGroups(key string, views ...string) string {
	var b strings.Builder
	b.WriteString("powerdns:\n  groups:\n")
	for _, p := range lab.PowerDNSes {
		fmt.Fprintf(&b, "    - name: %s\n      views: [%s]\n      primary: {url: %q, api_key: %q}\n",
			p.Group, strings.Join(views, ", "), p.URL(), key)
	}
	return b.String()
}

// TestPowerDNSCommands runs the powerdns commands against the lab's server
// groups.
func TestPowerDNSCommands(t *testing.T) {
	fixtures := map[string]*lab.PowerDNSFixture{}
	for _, p := range lab.PowerDNSes {
		fixtures[p.Group] = lab.NewPowerDNSFixture(t, p)
	}
	env := configFile(t, labGroups(lab.PowerDNSAPIKey, "_default_"))

	t.Run("check", func(t *testing.T) {
		code, out, stderr := run(t, env, "powerdns", "check", "-o", "json")
		var r powerDNSReport
		if err := json.Unmarshal([]byte(out), &r); err != nil || code != exitOK {
			t.Fatalf("exit %d, %v: %s\n%s", code, err, out, stderr)
		}
		if !r.OK || len(r.Groups) != len(lab.PowerDNSes) {
			t.Fatalf("report: %+v", r)
		}
		for i, p := range lab.PowerDNSes {
			g := r.Groups[i]
			names := make([]string, len(g.Checks))
			for j, c := range g.Checks {
				names[j] = c.Name
			}
			if g.Group != p.Group || !g.OK || !strings.HasPrefix(g.Version, p.Version+".") ||
				!slices.Equal(names, []string{"connection", "server", "key", "zones"}) || g.Checks[0].Result != checkWarning {
				t.Errorf("group %+v", g)
			}
		}
	})

	t.Run("check with a wrong key", func(t *testing.T) {
		code, out, stderr := run(t, configFile(t, labGroups("not-the-key", "_default_")), "powerdns", "check", "--group", "lab-a")
		if code != exitError || !strings.Contains(out, "PowerDNS rejected the API key of server group lab-a") ||
			!strings.Contains(stderr, "1 of 2 checks failed") {
			t.Errorf("exit %d:\n%s\n%s", code, out, stderr)
		}
	})

	t.Run("zones", func(t *testing.T) {
		code, out, stderr := run(t, env, "powerdns", "zones", "-o", "json")
		var zones []groupZone
		if err := json.Unmarshal([]byte(out), &zones); err != nil || code != exitOK {
			t.Fatalf("exit %d, %v: %s\n%s", code, err, out, stderr)
		}
		for group, f := range fixtures {
			for _, name := range append([]string{f.Zone}, f.Others...) {
				if !slices.ContainsFunc(zones, func(z groupZone) bool { return z.Group == group && z.Name == name && z.Kind == "Native" }) {
					t.Errorf("no zone %s in %s: %+v", name, group, zones)
				}
			}
		}
		code, out, _ = run(t, env, "powerdns", "zones", "--group", "lab-a")
		if code != exitOK || !strings.Contains(out, fixtures["lab-a"].Zone) {
			t.Errorf("exit %d:\n%s", code, out)
		}
	})

	t.Run("records", func(t *testing.T) {
		f := fixtures["lab-a"]
		code, out, stderr := run(t, env, "powerdns", "records", "--group", "lab-a", "--zone", strings.ToUpper(strings.TrimSuffix(f.Zone, ".")), "-o", "json")
		var gr struct {
			Group string `json:"group"`
			dns.Zone
			Problems []dns.Problem `json:"problems"`
		}
		if err := json.Unmarshal([]byte(out), &gr); err != nil || code != exitOK {
			t.Fatalf("exit %d, %v: %s\n%s", code, err, out, stderr)
		}
		if gr.Group != "lab-a" || gr.Name != f.Zone || len(gr.RRsets) != len(f.RRsets) || gr.Problems == nil || len(gr.Problems) != 0 ||
			strings.Contains(out, `"view"`) {
			t.Errorf("zone %s in %s, with %d RRsets and problems %v:\n%s", gr.Name, gr.Group, len(gr.RRsets), gr.Problems, out)
		}
		code, out, _ = run(t, env, "powerdns", "records", "--group", "lab-a", "--zone", f.Zone)
		for _, row := range []string{"off." + f.Zone, "192.0.2.12", "disabled", `"h3,h2"`} {
			if code != exitOK || !strings.Contains(out, row) {
				t.Errorf("table has no %q:\n%s", row, out)
			}
		}
	})

	t.Run("records of a missing zone", func(t *testing.T) {
		code, _, stderr := run(t, env, "powerdns", "records", "--group", "lab-a", "--zone", "missing.nbpdns.example")
		if code != exitError || !strings.Contains(stderr, "the primary of server group lab-a has no zone missing.nbpdns.example.") {
			t.Errorf("exit %d:\n%s", code, stderr)
		}
	})
}

// TestNetBoxZonesOfAGroup lists the NetBox zones that a server group serves,
// through its views.
func TestNetBoxZonesOfAGroup(t *testing.T) {
	nb := lab.NetBoxes[0]
	f := lab.NewFixture(t, nb)
	for _, tt := range []struct {
		views     []string
		wantViews []string
		warns     bool
	}{
		{[]string{f.View}, []string{f.View}, false},
		{[]string{f.View, f.OtherView}, []string{f.View, f.OtherView}, true},
	} {
		env := configFile(t, labGroups(lab.PowerDNSAPIKey, tt.views...))
		env["NBPDNS_NETBOX_URL"], env["NBPDNS_NETBOX_TOKEN"] = nb.URL(), f.ReaderToken
		code, out, stderr := run(t, env, "netbox", "zones", "--group", "lab-a", "-o", "json")
		var zones []dns.Zone
		if err := json.Unmarshal([]byte(out), &zones); err != nil || code != exitOK {
			t.Fatalf("exit %d, %v: %s\n%s", code, err, out, stderr)
		}
		var views []string
		for _, z := range zones {
			if z.Name != f.Zone+"." {
				t.Errorf("zone %s isn't in the group's views", z.Name)
			}
			views = append(views, z.View)
		}
		slices.Sort(views)
		want := slices.Sorted(slices.Values(tt.wantViews))
		if !slices.Equal(views, want) {
			t.Errorf("views %v, want %v", views, want)
		}
		if warned := strings.Contains(stderr, "serves two zones of one name"); warned != tt.warns {
			t.Errorf("warned %v, want %v:\n%s", warned, tt.warns, stderr)
		}
	}
}
