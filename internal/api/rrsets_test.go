package api

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/api/gen"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/service"
)

func active(value string) dns.Record { return dns.Record{Value: value, Status: "active", Active: true} }

func managed(value string) dns.Record {
	return dns.Record{Value: value, Status: "active", Active: true, Managed: true}
}

func inactive(value string) dns.Record { return dns.Record{Value: value, Status: "inactive"} }

// recorded is zoned, with NetBox's zones as of its last read: site-a serves
// _default_ and internal.
func recorded() fakeSource {
	src := zoned()
	zones := []dns.Zone{
		{Name: "example.com.", View: "_default_", Active: true, RRsets: []dns.RRset{
			{Name: "example.com.", Type: "SOA", TTL: 3600, Records: []dns.Record{managed("ns1.example.com. hostmaster.example.com. 1 10800 3600 604800 3600")}},
			{Name: "example.com.", Type: "NS", TTL: 3600, Records: []dns.Record{managed("ns1.example.com.")}},
			{Name: "mail.example.com.", Type: "A", TTL: 300, Records: []dns.Record{active("192.0.2.25")}},
			{Name: "old.example.com.", Type: "A", TTL: 300, Records: []dns.Record{inactive("192.0.2.1")}},
			{Name: "www.example.com.", Type: "A", TTL: 300, Records: []dns.Record{active("192.0.2.10"), inactive("192.0.2.11")}},
		}},
		{Name: "a.example.", View: "_default_", Active: true, RRsets: []dns.RRset{
			{Name: "a.example.", Type: "SOA", TTL: 3600, Records: []dns.Record{managed("ns1.a.example. h.a.example. 7 1 1 1 1")}},
		}},
		{Name: "dup.example.", View: "internal", Active: true, RRsets: []dns.RRset{
			{Name: "dup.example.", Type: "TXT", TTL: 60, Records: []dns.Record{active(`"internal"`)}},
		}},
		{Name: "dup.example.", View: "_default_", Active: true, RRsets: []dns.RRset{
			{Name: "dup.example.", Type: "TXT", TTL: 60, Records: []dns.Record{active(`"default"`)}},
		}},
		{Name: "f.example.", View: "elsewhere", Active: true},
		// Inactive in _default_, which the report compares, and active in
		// internal: the records aren't internal's.
		{Name: "split.example.", View: "_default_"},
		{Name: "split.example.", View: "internal", Active: true, RRsets: []dns.RRset{
			{Name: "split.example.", Type: "TXT", TTL: 60, Records: []dns.Record{active(`"internal"`)}},
		}},
	}
	src.netbox = service.NetBoxView{AsOf: at("2026-10-08T01:10:00Z"), Zones: map[string]map[string]dns.Zone{}}
	for _, z := range zones {
		if src.netbox.Zones[z.View] == nil {
			src.netbox.Zones[z.View] = map[string]dns.Zone{}
		}
		src.netbox.Zones[z.View][z.Name] = z
	}
	return src
}

func TestListZoneRRsets(t *testing.T) {
	r := get(t, recorded(), http.MethodGet, "/api/server-groups/site-a/zones/example.com/rrsets")
	page := decode[gen.RRsetPage](t, r)
	if r.code != http.StatusOK || page.Next != nil || page.AsOf != at("2026-10-08T01:10:00Z") {
		t.Fatalf("%d: %s", r.code, r.body)
	}
	var got []string
	for _, s := range page.Items {
		var values []string
		for _, rec := range s.Records {
			v := rec.Value
			if rec.Managed {
				v += " (managed)"
			}
			values = append(values, v)
		}
		got = append(got, s.Name+" "+s.Type+" "+strings.Join(values, ","))
	}
	// Only active records; the RRset with none goes.
	want := []string{
		"example.com. SOA ns1.example.com. hostmaster.example.com. 1 10800 3600 604800 3600 (managed)",
		"example.com. NS ns1.example.com. (managed)",
		"mail.example.com. A 192.0.2.25",
		"www.example.com. A 192.0.2.10",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("rrsets:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// A zone missing on the primary has its records, from NetBox.
	if page := decode[gen.RRsetPage](t, get(t, recorded(), http.MethodGet, "/api/server-groups/site-a/zones/a.example/rrsets")); len(page.Items) != 1 {
		t.Errorf("a.example.: %+v", page.Items)
	}
	// A zone in two of the group's views is the first view's, by name.
	if page := decode[gen.RRsetPage](t, get(t, recorded(), http.MethodGet, "/api/server-groups/site-a/zones/dup.example/rrsets")); len(page.Items) != 1 ||
		page.Items[0].Records[0].Value != `"default"` {
		t.Errorf("dup.example.: %+v", page.Items)
	}
}

func TestZoneRRsetPages(t *testing.T) {
	var types []string
	target := "/api/server-groups/site-a/zones/example.com./rrsets?limit=1"
	for target != "" && len(types) < 10 {
		page := decode[gen.RRsetPage](t, get(t, recorded(), http.MethodGet, target))
		for _, s := range page.Items {
			types = append(types, s.Name+"/"+s.Type)
		}
		target = ""
		if page.Next != nil {
			u, err := url.Parse(*page.Next)
			if err != nil {
				t.Fatal(err)
			}
			target = u.RequestURI()
		}
	}
	if got := strings.Join(types, " "); got != "example.com./SOA example.com./NS mail.example.com./A www.example.com./A" {
		t.Errorf("paged %s", got)
	}
	problemOf(t, get(t, recorded(), http.MethodGet, "/api/server-groups/site-a/zones/example.com/rrsets?limit=1001"), http.StatusBadRequest)
}

func TestZoneRRsetsNotFound(t *testing.T) {
	never := recorded()
	never.netbox = service.NetBoxView{}
	tests := []struct {
		name   string
		src    fakeSource
		path   string
		detail string
	}{
		{"NetBox never read", never, "/api/server-groups/site-a/zones/example.com/rrsets", "NetBox hasn't been read yet"},
		{"an unmanaged zone", recorded(), "/api/server-groups/site-a/zones/z.example/rrsets", "NetBox has no active zone z.example. in server group site-a's views, _default_, internal."},
		{"a zone in another view", recorded(), "/api/server-groups/site-a/zones/f.example/rrsets", "no active zone f.example."},
		{"a zone inactive in the view compared", recorded(), "/api/server-groups/site-a/zones/split.example/rrsets", "no active zone split.example."},
		{"an unknown group", recorded(), "/api/server-groups/site-z/zones/example.com/rrsets", "No server group is named site-z."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if p := problemOf(t, get(t, tt.src, http.MethodGet, tt.path), http.StatusNotFound); !strings.Contains(*p.Detail, tt.detail) {
				t.Errorf("detail %q, want %q", *p.Detail, tt.detail)
			}
		})
	}
}

func TestZoneRRsetCount(t *testing.T) {
	_, page := zoneNames(t, get(t, recorded(), http.MethodGet, "/api/server-groups/site-a/zones"))
	counts := map[string]string{}
	for _, z := range page.Items {
		c := "null"
		if z.RrsetCount != nil {
			c = strconv.FormatInt(*z.RrsetCount, 10)
		}
		counts[z.Zone] = c
	}
	if counts["example.com."] != "4" || counts["a.example."] != "1" || counts["z.example."] != "null" || counts["c.example."] != "null" {
		t.Errorf("rrset counts %v", counts)
	}
	z := decode[gen.ZoneDetail](t, get(t, recorded(), http.MethodGet, "/api/server-groups/site-a/zones/example.com"))
	if z.RrsetCount == nil || *z.RrsetCount != 4 {
		t.Errorf("example.com.'s rrset_count %v", z.RrsetCount)
	}
}
