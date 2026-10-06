package powerdns

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/lab"
)

// checkFixtureZone checks z, the lab fixture's main zone read into the
// normalized model, and the problems found, against what the fixture
// created. The unit tests run it on responses recorded from the lab, and
// the integration tests on the lab itself.
func checkFixtureZone(t *testing.T, f *lab.PowerDNSFixture, z dns.Zone, probs []dns.Problem) {
	t.Helper()
	zone, ns := f.Zone, f.Nameserver
	if z.Name != zone || z.View != "" || z.DefaultTTL != 0 || !z.Active || z.SOASerial == 0 ||
		!slices.Equal(z.Nameservers, []string{ns}) {
		z.RRsets = nil
		t.Errorf("zone = %+v", z)
	}
	hex := "ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789"
	active := func(values ...string) []dns.Record {
		out := make([]dns.Record, len(values))
		for i, v := range values {
			out[i] = dns.Record{Value: v, TTL: 3600, Status: "active", Active: true}
		}
		return out
	}
	long := strings.Repeat("a", 300)
	tests := []struct {
		name, typ string
		ttl       uint32
		records   []dns.Record
	}{
		{zone, "NS", 3600, active(ns)},
		// The disabled record doesn't count towards the RRset's TTL, which
		// PowerDNS keeps one per RRset anyway.
		{"www." + zone, "A", 300, []dns.Record{
			{Value: "192.0.2.10", TTL: 300, Status: "active", Active: true},
			{Value: "192.0.2.11", TTL: 300, Status: "active", Active: true},
			{Value: "192.0.2.14", TTL: 300, Status: "disabled"},
		}},
		{"www." + zone, "AAAA", 3600, active("2001:db8::10")},
		{"off." + zone, "A", 3600, []dns.Record{{Value: "192.0.2.12", TTL: 3600, Status: "disabled"}}},
		{"alias." + zone, "CNAME", 3600, active("www." + zone)},
		{zone, "MX", 3600, active("10 mail." + zone)},
		{"_sip._tcp." + zone, "SRV", 3600, active("10 5 5060 sip." + zone)},
		{zone, "TXT", 3600, active(`"v=spf1 -all"`)},
		{"quoted." + zone, "TXT", 3600, active(`"quoted" "two"`)},
		{"long." + zone, "TXT", 3600, active(`"` + long[:255] + `" "` + long[255:] + `"`)},
		{"xn--bcher-kva." + zone, "A", 3600, active("192.0.2.13")},
		{"caa." + zone, "CAA", 3600, active(`0 issue "letsencrypt.org"`)},
		{"_443._tcp.www." + zone, "TLSA", 3600, active("3 1 1 " + hex)},
		{"www." + zone, "HTTPS", 3600, active(`1 . alpn="h3,h2" ipv4hint="192.0.2.10"`)},
		{zone, "SSHFP", 3600, active("4 2 " + hex)},
		{"svc." + zone, "SVCB", 3600, active(`1 web.` + zone + ` port="8443"`)},
		{"naptr." + zone, "NAPTR", 3600, active(`100 10 "S" "SIP+D2U" "" _sip._udp.` + zone)},
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
	// The SOA record comes first, in canonical order. PowerDNS may change
	// its serial, so only its names are checked.
	if len(z.RRsets) == 0 || z.RRsets[0].Type != "SOA" || z.RRsets[0].Name != zone {
		t.Errorf("the first RRset isn't the zone's SOA: %+v", z.RRsets[:min(len(z.RRsets), 1)])
	} else if soa := z.RRsets[0].Records; len(soa) != 1 || soa[0].Managed || !strings.HasPrefix(soa[0].Value, ns+" hostmaster."+zone+" ") {
		t.Errorf("SOA = %+v", soa)
	}
	if len(z.RRsets) != len(f.RRsets) {
		t.Errorf("read %d RRsets, want %d", len(z.RRsets), len(f.RRsets))
	}
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
