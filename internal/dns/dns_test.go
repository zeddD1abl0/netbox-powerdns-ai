package dns

import (
	"cmp"
	"reflect"
	"strings"
	"testing"
)

func TestName(t *testing.T) {
	tests := []struct{ name, origin, want string }{
		{"www", "example.com", "www.example.com."},
		{"www", "example.com.", "www.example.com."},
		{"WWW.Example.COM.", "example.com", "www.example.com."},
		{"@", "Example.com", "example.com."},
		{"", "example.com.", "example.com."},
		{" mail ", "example.com", "mail.example.com."},
		{"_sip._tcp", "example.com", "_sip._tcp.example.com."},
		{"xn--bcher-kva", "example", "xn--bcher-kva.example."},
		{"com", ".", "com."},
		{".", "example.com", "."},
	}
	for _, tt := range tests {
		if got := Name(tt.name, tt.origin); got != tt.want {
			t.Errorf("Name(%q, %q) = %q, want %q", tt.name, tt.origin, got, tt.want)
		}
	}
}

func TestCompareNames(t *testing.T) {
	sorted := []string{"example.com.", "_sip._tcp.example.com.", "a.example.com.", "z.a.example.com.", "b.example.com.", "example.org."}
	for i, a := range sorted {
		for j, b := range sorted {
			if got, want := CompareNames(a, b), cmp.Compare(i, j); got != want {
				t.Errorf("CompareNames(%q, %q) = %d, want %d", a, b, got, want)
			}
		}
	}
}

func TestValue(t *testing.T) {
	long := strings.Repeat("a", 300)
	tests := []struct {
		typ, value, want string
		wantErr          bool
	}{
		{"A", "192.0.2.10", "192.0.2.10", false},
		{"A", " 192.0.2.10 ", "192.0.2.10", false},
		{"A", "2001:db8::1", "2001:db8::1", true},
		{"A", "300.1.1.1", "300.1.1.1", true},
		{"AAAA", "2001:DB8:0:0:0:0:0:1", "2001:db8::1", false},
		{"AAAA", "192.0.2.1", "192.0.2.1", true},
		{"CNAME", "www", "www.example.com.", false},
		{"CNAME", "Target.Example.ORG.", "target.example.org.", false},
		{"CNAME", "two names", "two names", true},
		{"DNAME", "other", "other.example.com.", false},
		{"NS", "ns1.example.net.", "ns1.example.net.", false},
		{"PTR", "host", "host.example.com.", false},
		{"MX", "10 mail", "10 mail.example.com.", false},
		{"MX", "010  Mail.Example.net.", "10 mail.example.net.", false},
		{"MX", "0 .", "0 .", false},
		{"MX", "high mail", "high mail", true},
		{"SRV", "10 5 5060 sip.example.org.", "10 5 5060 sip.example.org.", false},
		{"SRV", "0 0 443 web", "0 0 443 web.example.com.", false},
		{"SRV", "1 2 70000 web", "1 2 70000 web", true},
		{"SOA", "ns1 hostmaster 1 43200 7200 2419200 3600", "ns1.example.com. hostmaster.example.com. 1 43200 7200 2419200 3600", false},
		{"TXT", "v=spf1 -all", `"v=spf1 -all"`, false},
		{"TXT", `"quoted" "two"`, `"quoted" "two"`, false},
		{"TXT", `"a b"   "c"`, `"a b" "c"`, false},
		{"TXT", `"with \"escape\" and \\"`, `"with \"escape\" and \\"`, false},
		{"TXT", `"decimal \065"`, `"decimal A"`, false},
		{"TXT", `he said "hi"`, `"he said \"hi\""`, false},
		{"TXT", "", `""`, false},
		{"TXT", "café", `"caf\195\169"`, false},
		{"TXT", long, `"` + long[:255] + `" "` + long[255:] + `"`, false},
		{"TXT", `"unclosed`, `"unclosed`, true},
		{"SPF", "v=spf1 -all", `"v=spf1 -all"`, false},
		{"CAA", `0  issue   "letsencrypt.org"`, `0 issue "letsencrypt.org"`, false},
	}
	for _, tt := range tests {
		got, err := Value(tt.typ, tt.value, "example.com.")
		if got != tt.want || (err != nil) != tt.wantErr {
			t.Errorf("Value(%s, %q) = %q, %v; want %q, error %v", tt.typ, tt.value, got, err, tt.want, tt.wantErr)
		}
	}
}

func u32(n uint32) *uint32 { return &n }

func TestSetRecords(t *testing.T) {
	z := Zone{Name: "example.com.", DefaultTTL: 3600}
	probs := z.SetRecords([]RawRecord{
		{Name: "www", Type: "a", Value: "192.0.2.11", TTL: u32(300), Status: "active", Active: true},
		{Name: "www", Type: "A", Value: "192.0.2.10", Status: "active", Active: true},
		{Name: "WWW.example.com.", Type: "A", Value: "192.0.2.12", TTL: u32(60), Status: "inactive"},
		{Name: "@", Type: "SOA", Value: "ns1.example.com. hostmaster.example.com. 1 2 3 4 5", TTL: u32(86400), Status: "active", Active: true, Managed: true},
		{Name: "@", Type: "NS", Value: "ns1.example.com.", Status: "active", Active: true, Managed: true},
		{Name: "@", Type: "MX", Value: "10 mail", Status: "active", Active: true},
		{Name: "old", Type: "A", Value: "192.0.2.20", TTL: u32(120), Status: "inactive"},
		{Name: "old", Type: "A", Value: "192.0.2.21", Status: "inactive"},
		{Name: "bad", Type: "A", Value: "not-an-address", Status: "active", Active: true},
		// The same value twice, active and inactive, sorts the same way
		// whatever order NetBox gives them in.
		{Name: "dup", Type: "A", Value: "192.0.2.30", TTL: u32(60), Status: "inactive"},
		{Name: "dup", Type: "A", Value: "192.0.2.30", Status: "active", Active: true},
	})
	want := []RRset{
		{Name: "example.com.", Type: "SOA", TTL: 86400, Records: []Record{{"ns1.example.com. hostmaster.example.com. 1 2 3 4 5", 86400, "active", true, true}}},
		{Name: "example.com.", Type: "NS", TTL: 3600, Records: []Record{{"ns1.example.com.", 3600, "active", true, true}}},
		{Name: "example.com.", Type: "MX", TTL: 3600, Records: []Record{{"10 mail.example.com.", 3600, "active", true, false}}},
		{Name: "bad.example.com.", Type: "A", TTL: 3600, Records: []Record{{"not-an-address", 3600, "active", true, false}}},
		{Name: "dup.example.com.", Type: "A", TTL: 3600, Records: []Record{
			{"192.0.2.30", 3600, "active", true, false},
			{"192.0.2.30", 60, "inactive", false, false},
		}},
		// Only inactive records: the lowest TTL of all of them.
		{Name: "old.example.com.", Type: "A", TTL: 120, Records: []Record{
			{"192.0.2.20", 120, "inactive", false, false},
			{"192.0.2.21", 3600, "inactive", false, false},
		}},
		// The inactive record's lower TTL doesn't count.
		{Name: "www.example.com.", Type: "A", TTL: 300, Records: []Record{
			{"192.0.2.10", 3600, "active", true, false},
			{"192.0.2.11", 300, "active", true, false},
			{"192.0.2.12", 60, "inactive", false, false},
		}},
	}
	if !reflect.DeepEqual(z.RRsets, want) {
		t.Errorf("RRsets:\n got %+v\nwant %+v", z.RRsets, want)
	}
	wantProbs := []string{
		"example.com. bad.example.com. A: \"not-an-address\"",
		"example.com. www.example.com. A: its active records have different TTLs, from 300 to 3600; the RRset uses 300",
	}
	if len(probs) != len(wantProbs) {
		t.Fatalf("problems = %v, want %d", probs, len(wantProbs))
	}
	for i, p := range probs {
		if !strings.HasPrefix(p.String(), wantProbs[i]) {
			t.Errorf("problem %d = %q, want it to start with %q", i, p, wantProbs[i])
		}
	}
}

// TestValueAcrossSources gives the same data as NetBox's DNS plugin keeps it,
// entered relative and in any case, and as PowerDNS keeps it, absolute and
// in its own text, and expects one value from both (ADR-0025).
func TestValueAcrossSources(t *testing.T) {
	const hex = "ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789"
	lower := strings.ToLower(hex)
	// Hex is compared in uppercase, which miekg/dns prints for some types
	// whatever case it's given.
	tests := []struct {
		typ, netbox, powerdns, want string
	}{
		{"CAA", `0 issue letsencrypt.org`, `0 issue "letsencrypt.org"`, `0 issue "letsencrypt.org"`},
		{"TLSA", "3 1 1 " + lower, "3 1 1 " + hex, "3 1 1 " + hex},
		{"SSHFP", "4 2 " + lower, "4 2 " + hex, "4 2 " + hex},
		{"DS", "12345 13 2 " + lower, "12345 13 2 " + hex, "12345 13 2 " + hex},
		{"HTTPS", `1 . alpn=h3,h2 ipv4hint=192.0.2.1`, `1 . alpn="h3,h2" ipv4hint=192.0.2.1`, `1 . alpn="h3,h2" ipv4hint="192.0.2.1"`},
		{"SVCB", `1 Svc port=8443`, `1 svc.example.com. port="8443"`, `1 svc.example.com. port="8443"`},
		{"NAPTR", `100 10 "S" "SIP+D2U" "" _sip._udp`, `100 10 "S" "SIP+D2U" "" _sip._udp.example.com.`,
			`100 10 "S" "SIP+D2U" "" _sip._udp.example.com.`},
		{"LOC", "52 22 23 N 4 53 32 E -2m 0m 10000m 10m", "52 22 23.000 N 4 53 32.000 E -2.00m 0.00m 10000.00m 10.00m",
			"52 22 23.000 N 04 53 32.000 E -2m 0.00m 10000m 10m"},
		{"TXT", "v=spf1 mx -all", `"v=spf1 mx -all"`, `"v=spf1 mx -all"`},
		{"MX", "10 Mail", "10 mail.example.com.", "10 mail.example.com."},
		{"SRV", "10 5 5060 SIP", "10 5 5060 sip.example.com.", "10 5 5060 sip.example.com."},
		{"SOA", "ns1 hostmaster 2026100601 43200 7200 2419200 3600",
			"ns1.example.com. hostmaster.example.com. 2026100601 43200 7200 2419200 3600",
			"ns1.example.com. hostmaster.example.com. 2026100601 43200 7200 2419200 3600"},
		{"AAAA", "2001:DB8::0:1", "2001:db8::1", "2001:db8::1"},
	}
	for _, tt := range tests {
		for _, in := range []string{tt.netbox, tt.powerdns} {
			if got, err := Value(tt.typ, in, "example.com."); got != tt.want || err != nil {
				t.Errorf("Value(%s, %q) = %q, %v; want %q", tt.typ, in, got, err, tt.want)
			}
		}
	}
}

// TestValueOutOfRange checks numbers too big for their fields, which
// miekg/dns would otherwise keep modulo the field's size.
func TestValueOutOfRange(t *testing.T) {
	tests := []struct{ typ, value string }{
		{"MX", "70000 mail"},
		{"SRV", "70000 1 1 web"},
		{"CAA", `300 issue "x"`},
		{"TLSA", "300 1 1 abcd"},
		{"DS", "70000 13 2 abcd"},
		{"HTTPS", "70000 . alpn=h2"},
		{"SOA", "ns1 h 99999999999 1 1 1 1"},
	}
	for _, tt := range tests {
		if got, err := Value(tt.typ, tt.value, "example.com."); err == nil || got != tt.value {
			t.Errorf("Value(%s, %q) = %q, %v; want it kept as given, with an error", tt.typ, tt.value, got, err)
		}
	}
	// Numbers written with leading zeros, or printed differently, aren't
	// out of range.
	if got, err := Value("MX", "010 mail", "example.com."); got != "10 mail.example.com." || err != nil {
		t.Errorf("Value(MX, 010 mail) = %q, %v", got, err)
	}
}
