package netbox

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/lab"
)

// checkFixtureZone checks z, the zone in f's view read into the normalized
// model, and the problems found, against what the lab fixture created. The
// unit tests run it on responses recorded from the lab, and the integration
// tests on the lab itself.
func checkFixtureZone(t *testing.T, f *lab.Fixture, z dns.Zone, probs []dns.Problem) {
	t.Helper()
	zone, ns := f.Zone+".", f.Nameserver+"."
	if z.Name != zone || z.View != f.View || z.Status != "active" || !z.Active || z.DefaultTTL != f.DefaultTTL ||
		z.SOASerial == 0 || !slices.Equal(z.Nameservers, []string{ns}) {
		z.RRsets = nil
		t.Errorf("zone = %+v", z)
	}

	ttl := f.DefaultTTL
	active := func(values ...string) []dns.Record {
		out := make([]dns.Record, len(values))
		for i, v := range values {
			out[i] = dns.Record{Value: v, TTL: ttl, Status: "active", Active: true}
		}
		return out
	}
	long := strings.Repeat("a", 300)
	tests := []struct {
		name, typ string
		ttl       uint32
		records   []dns.Record
	}{
		{zone, "NS", ttl, []dns.Record{{Value: ns, TTL: ttl, Status: "active", Active: true, Managed: true}}},
		// The second active record took the first's TTL in NetBox, and the
		// inactive record's own TTL doesn't count.
		{"www." + zone, "A", 300, []dns.Record{
			{Value: "192.0.2.10", TTL: 300, Status: "active", Active: true},
			{Value: "192.0.2.11", TTL: 300, Status: "active", Active: true},
			{Value: "192.0.2.14", TTL: 60, Status: "inactive"},
		}},
		{"www." + zone, "AAAA", ttl, active("2001:db8::10")},
		{"mixed." + zone, "A", ttl, []dns.Record{{Value: "192.0.2.12", TTL: ttl, Status: "inactive"}}},
		{"alias." + zone, "CNAME", ttl, active("www." + zone)},
		{zone, "MX", ttl, active("10 mail." + zone)},
		{"_sip._tcp." + zone, "SRV", ttl, active("10 5 5060 sip." + zone)},
		{zone, "TXT", ttl, active(`"v=spf1 -all"`)},
		{"quoted." + zone, "TXT", ttl, active(`"quoted" "two"`)},
		// The plugin splits a long value into 255-byte strings itself.
		{"long." + zone, "TXT", ttl, active(`"` + long[:255] + `" "` + long[255:] + `"`)},
		{"xn--bcher-kva." + zone, "A", ttl, active("192.0.2.13")},
		{"caa." + zone, "CAA", ttl, active(`0 issue "letsencrypt.org"`)},
		{"host-00." + zone, "A", ttl, active("198.51.100.1")},
		{"host-23." + zone, "A", ttl, active("198.51.100.24")},
	}
	for _, tt := range tests {
		s := findRRset(z, tt.name, tt.typ)
		if s == nil {
			t.Errorf("no %s %s RRset", tt.name, tt.typ)
			continue
		}
		if s.TTL != tt.ttl || !reflect.DeepEqual(s.Records, tt.records) {
			t.Errorf("%s %s:\n got TTL %d, %+v\nwant TTL %d, %+v", tt.name, tt.typ, s.TTL, s.Records, tt.ttl, tt.records)
		}
	}

	// The plugin's SOA record comes first, in canonical order.
	if len(z.RRsets) == 0 || z.RRsets[0].Type != "SOA" || z.RRsets[0].Name != zone {
		t.Errorf("the first RRset isn't the zone's SOA: %+v", z.RRsets[:min(len(z.RRsets), 1)])
	} else if soa := z.RRsets[0].Records; len(soa) != 1 || !soa[0].Managed || !soa[0].Active ||
		!strings.HasPrefix(soa[0].Value, ns+" hostmaster."+zone+" ") {
		t.Errorf("SOA = %+v", soa)
	}

	// Every record, the plugin's SOA and NS records included, across
	// several pages.
	n := 0
	for _, s := range z.RRsets {
		n += len(s.Records)
	}
	if want := len(f.Records) + 2; n != want {
		t.Errorf("read %d records, want %d", n, want)
	}

	// The plugin's default settings keep the data consistent, so there's
	// nothing for normalization to work around.
	if len(probs) != 0 {
		t.Errorf("problems = %v, want none", probs)
	}
}

func findRRset(z dns.Zone, name, typ string) *dns.RRset {
	for i, s := range z.RRsets {
		if s.Name == name && s.Type == typ {
			return &z.RRsets[i]
		}
	}
	return nil
}
