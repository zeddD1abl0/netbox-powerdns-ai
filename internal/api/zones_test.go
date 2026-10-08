package api

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/api/gen"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
)

// zoned is site-a with a zone in each state, and site-c, never read.
func zoned() fakeSource {
	src := threeGroups()
	src.groups[0].Report = &drift.GroupReport{
		Group: "site-a", Status: drift.StatusOK,
		Zones: []drift.ZoneReport{
			{Zone: "a.example.", View: "_default_", Policy: "report", State: drift.StateMissing, NetBoxSerial: 7},
			{Zone: "example.com.", View: "_default_", Policy: "report", State: drift.StateDrift, NetBoxSerial: 2026100801, PowerDNSSerial: 2026100701,
				Changes: []drift.Change{
					{Name: "example.com.", Type: "SOA", Kind: drift.ChangeChanged, NetBox: &drift.Side{TTL: 3600, Values: []string{"ns1. h. 2 1 1 1 1"}}, PowerDNS: &drift.Side{TTL: 3600, Values: []string{"ns1. h. 1 1 1 1 1"}}},
					{Name: "mail.example.com.", Type: "A", Kind: drift.ChangeMissing, NetBox: &drift.Side{TTL: 300, Values: []string{"192.0.2.25"}}},
					{Name: "www.example.com.", Type: "A", Kind: drift.ChangeExtra, PowerDNS: &drift.Side{TTL: 300, Values: []string{"192.0.2.99"}}},
				}},
			{Zone: "b.example.", View: "internal", Policy: "enforce", State: drift.StateInactive, PowerDNSSerial: 3},
			{Zone: "c.example.", View: "_default_", Policy: "report", State: drift.StateInSync, NetBoxSerial: 1, PowerDNSSerial: 1},
			{Zone: "d.example.", View: "_default_", Policy: "ignore", State: drift.StateIgnored},
		},
		Unmanaged: []string{"z.example.", "0.example."},
	}
	return src
}

func zoneNames(t *testing.T, r reply) ([]string, gen.ZonePage) {
	t.Helper()
	if r.code != http.StatusOK {
		t.Fatalf("%d: %s", r.code, r.body)
	}
	page := decode[gen.ZonePage](t, r)
	var names []string
	for _, z := range page.Items {
		names = append(names, z.Zone)
	}
	return names, page
}

func TestListZones(t *testing.T) {
	names, page := zoneNames(t, get(t, zoned(), http.MethodGet, "/api/server-groups/site-a/zones"))
	if got := strings.Join(names, " "); got != "example.com. 0.example. a.example. b.example. c.example. d.example. z.example." {
		t.Errorf("zones %s, want canonical order", got)
	}
	if page.AsOf == nil || page.Next != nil {
		t.Errorf("as_of %v, next %v", page.AsOf, page.Next)
	}
	byName := map[string]gen.Zone{}
	for _, z := range page.Items {
		byName[z.Zone] = z
	}
	if z := byName["z.example."]; z.State != gen.ZoneStateUnmanaged || z.View != nil || z.Policy != nil || z.NetboxSerial != nil {
		t.Errorf("unmanaged: %+v", z)
	}
	if z := byName["example.com."]; z.State != gen.ZoneStateDrift || *z.View != "_default_" || *z.Policy != gen.DriftPolicyReport ||
		*z.NetboxSerial != 2026100801 || *z.PowerdnsSerial != 2026100701 || z.ChangeCount != 3 {
		t.Errorf("drifted: %+v", z)
	}
	if z := byName["a.example."]; z.PowerdnsSerial != nil || *z.NetboxSerial != 7 {
		t.Errorf("missing: %+v", z)
	}

	// A group never read has no zones yet.
	names, page = zoneNames(t, get(t, zoned(), http.MethodGet, "/api/server-groups/site-c/zones"))
	if len(names) != 0 || page.AsOf != nil {
		t.Errorf("site-c: %v as of %v", names, page.AsOf)
	}
	problemOf(t, get(t, zoned(), http.MethodGet, "/api/server-groups/site-z/zones"), http.StatusNotFound)
}

func TestZoneStateFilter(t *testing.T) {
	tests := []struct {
		query, want string
	}{
		{"state=drift", "example.com."},
		{"state=drift,missing", "example.com. a.example."},
		{"state=missing,drift,missing", "example.com. a.example."},
		{"state=unmanaged", "0.example. z.example."},
		{"state=in_sync,ignored,inactive_in_netbox", "b.example. c.example. d.example."},
	}
	for _, tt := range tests {
		names, _ := zoneNames(t, get(t, zoned(), http.MethodGet, "/api/server-groups/site-a/zones?"+tt.query))
		if got := strings.Join(names, " "); got != tt.want {
			t.Errorf("%s: %s, want %s", tt.query, got, tt.want)
		}
	}
	p := problemOf(t, get(t, zoned(), http.MethodGet, "/api/server-groups/site-a/zones?state=drifty"), http.StatusBadRequest)
	if !strings.Contains(*p.Detail, "drifty") {
		t.Errorf("detail %q", *p.Detail)
	}
}

// zonePages follows the zones' next links from target, and returns each
// page's zones.
func zonePages(t *testing.T, src Source, target string) []string {
	t.Helper()
	var out []string
	for target != "" && len(out) < 20 {
		names, page := zoneNames(t, get(t, src, http.MethodGet, target))
		out = append(out, strings.Join(names, " "))
		target = ""
		if page.Next != nil {
			u, err := url.Parse(*page.Next)
			if err != nil {
				t.Fatal(err)
			}
			target = u.RequestURI()
		}
	}
	return out
}

func TestZonePages(t *testing.T) {
	got := zonePages(t, zoned(), "/api/server-groups/site-a/zones?limit=3")
	if strings.Join(got, " | ") != "example.com. 0.example. a.example. | b.example. c.example. d.example. | z.example." {
		t.Errorf("pages %q", got)
	}
	// With a filter, each next link keeps it.
	got = zonePages(t, zoned(), "/api/server-groups/site-a/zones?state=missing,drift,unmanaged&limit=2")
	if strings.Join(got, " | ") != "example.com. 0.example. | a.example. z.example." {
		t.Errorf("filtered pages %q", got)
	}
	// A cursor made with one filter isn't good with another, or none.
	_, page := zoneNames(t, get(t, zoned(), http.MethodGet, "/api/server-groups/site-a/zones?state=drift,missing&limit=1"))
	u, err := url.Parse(*page.Next)
	if err != nil {
		t.Fatal(err)
	}
	c := u.Query().Get("cursor")
	for _, q := range []string{"state=missing&cursor=" + c, "cursor=" + c} {
		problemOf(t, get(t, zoned(), http.MethodGet, "/api/server-groups/site-a/zones?"+q), http.StatusBadRequest)
	}
	// The same filter, given in another order, is the same filter.
	if names, _ := zoneNames(t, get(t, zoned(), http.MethodGet, "/api/server-groups/site-a/zones?state=missing,drift&cursor="+c)); strings.Join(names, " ") != "a.example." {
		t.Errorf("after the cursor: %v", names)
	}
}

// A refresh between pages drops or adds zones; the cursor goes on after the
// zone it names, whether it's still there or not.
func TestZonePagesAcrossARefresh(t *testing.T) {
	_, page := zoneNames(t, get(t, zoned(), http.MethodGet, "/api/server-groups/site-a/zones?limit=3"))
	u, err := url.Parse(*page.Next)
	if err != nil {
		t.Fatal(err)
	}
	after := zoned()
	r := *after.groups[0].Report
	r.Zones = r.Zones[1:] // a.example., the cursor's zone, is gone
	after.groups[0].Report = &r
	if names, _ := zoneNames(t, get(t, after, http.MethodGet, u.RequestURI())); strings.Join(names, " ") != "b.example. c.example. d.example." {
		t.Errorf("after the refresh: %v", names)
	}
}

func TestGetZone(t *testing.T) {
	for _, name := range []string{"example.com", "example.com.", "EXAMPLE.com"} {
		r := get(t, zoned(), http.MethodGet, "/api/server-groups/site-a/zones/"+name)
		if z := decode[gen.ZoneDetail](t, r); r.code != http.StatusOK || z.Zone != "example.com." || z.ChangeCount != 3 || z.AsOf.IsZero() {
			t.Errorf("%s: %d %s", name, r.code, r.body)
		}
	}
	r := get(t, zoned(), http.MethodGet, "/api/server-groups/site-a/zones/z.example")
	if z := decode[gen.ZoneDetail](t, r); r.code != http.StatusOK || z.State != gen.ZoneStateUnmanaged || z.Policy != nil {
		t.Errorf("unmanaged: %d %s", r.code, r.body)
	}
	tests := []struct {
		path, detail string
	}{
		{"/api/server-groups/site-a/zones/nope.example", "Server group site-a has no zone nope.example.."},
		{"/api/server-groups/site-c/zones/example.com", "hasn't been read yet"},
		{"/api/server-groups/site-z/zones/example.com", "No server group is named site-z."},
	}
	for _, tt := range tests {
		if p := problemOf(t, get(t, zoned(), http.MethodGet, tt.path), http.StatusNotFound); !strings.Contains(*p.Detail, tt.detail) {
			t.Errorf("%s: detail %q", tt.path, *p.Detail)
		}
	}
}

func TestListZoneChanges(t *testing.T) {
	r := get(t, zoned(), http.MethodGet, "/api/server-groups/site-a/zones/example.com/changes")
	page := decode[gen.ChangePage](t, r)
	if r.code != http.StatusOK || len(page.Items) != 3 || page.Next != nil || page.AsOf.IsZero() {
		t.Fatalf("%d: %s", r.code, r.body)
	}
	soa, missing, extra := page.Items[0], page.Items[1], page.Items[2]
	if soa.Type != "SOA" || soa.Kind != gen.ChangeKindChanged || soa.Netbox == nil || soa.Powerdns == nil {
		t.Errorf("SOA: %+v", soa)
	}
	if missing.Kind != gen.ChangeKindMissing || missing.Powerdns != nil || missing.Netbox.Ttl != 300 || missing.Netbox.Values[0] != "192.0.2.25" {
		t.Errorf("missing: %+v", missing)
	}
	if extra.Kind != gen.ChangeKindExtra || extra.Netbox != nil || !strings.Contains(string(r.body), `"netbox":null`) {
		t.Errorf("extra: %+v", extra)
	}

	// Paged one at a time, the cursor names the last RRset's name and type.
	var kinds []string
	target := "/api/server-groups/site-a/zones/example.com./changes?limit=1"
	for target != "" && len(kinds) < 10 {
		page := decode[gen.ChangePage](t, get(t, zoned(), http.MethodGet, target))
		for _, c := range page.Items {
			kinds = append(kinds, string(c.Kind))
		}
		target = ""
		if page.Next != nil {
			u, _ := url.Parse(*page.Next)
			target = u.RequestURI()
		}
	}
	if strings.Join(kinds, " ") != "changed missing extra" {
		t.Errorf("paged kinds %v", kinds)
	}

	// Zones without changes have none; unknown zones are 404s.
	for _, zone := range []string{"c.example", "z.example", "d.example"} {
		if page := decode[gen.ChangePage](t, get(t, zoned(), http.MethodGet, "/api/server-groups/site-a/zones/"+zone+"/changes")); len(page.Items) != 0 {
			t.Errorf("%s: %+v", zone, page.Items)
		}
	}
	problemOf(t, get(t, zoned(), http.MethodGet, "/api/server-groups/site-a/zones/nope.example/changes"), http.StatusNotFound)
	problemOf(t, get(t, zoned(), http.MethodGet, "/api/server-groups/site-a/zones/example.com/changes?limit=0"), http.StatusBadRequest)
}

// A zone named with a /, as RFC 2317's classless reverse zones are, keeps
// its escape in the links, so its pages can be followed.
func TestEscapedZoneLinks(t *testing.T) {
	src := zoned()
	r := *src.groups[0].Report
	r.Zones = append(r.Zones, drift.ZoneReport{Zone: "0/25.2.0.192.in-addr.arpa.", View: "_default_", Policy: "report", State: drift.StateDrift,
		Changes: []drift.Change{
			{Name: "1.0/25.2.0.192.in-addr.arpa.", Type: "PTR", Kind: drift.ChangeMissing, NetBox: &drift.Side{TTL: 300, Values: []string{"a.example."}}},
			{Name: "2.0/25.2.0.192.in-addr.arpa.", Type: "PTR", Kind: drift.ChangeMissing, NetBox: &drift.Side{TTL: 300, Values: []string{"b.example."}}},
		}})
	src.groups[0].Report = &r
	target := "/api/server-groups/site-a/zones/0%2F25.2.0.192.in-addr.arpa/changes?limit=1"
	var names []string
	for target != "" && len(names) < 5 {
		page := decode[gen.ChangePage](t, get(t, src, http.MethodGet, target))
		for _, c := range page.Items {
			names = append(names, c.Name)
		}
		target = ""
		if page.Next != nil {
			if !strings.Contains(*page.Next, "/zones/0%2F25.2.0.192.in-addr.arpa/changes?") {
				t.Fatalf("next %q lost the escape", *page.Next)
			}
			u, err := url.Parse(*page.Next)
			if err != nil {
				t.Fatal(err)
			}
			target = u.RequestURI()
		}
	}
	if strings.Join(names, " ") != "1.0/25.2.0.192.in-addr.arpa. 2.0/25.2.0.192.in-addr.arpa." {
		t.Errorf("changes %v", names)
	}
}
