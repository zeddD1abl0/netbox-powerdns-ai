package lab

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// A PowerDNSFixture is DNS data created in a lab PowerDNS server for one
// test. Every name in it contains its ID, so fixtures of concurrent tests
// don't collide.
type PowerDNSFixture struct {
	ID string
	// Zone is the main zone's name, such as t1a2b3c4d.nbpdns.example., which
	// holds RRsets.
	Zone string
	// Others are two more zones, each holding only its SOA and NS records.
	Others []string
	// Nameserver is every zone's name server, outside the zones.
	Nameserver string
	// RRsets are the main zone's RRsets, as the fixture creates them.
	RRsets []FixtureRRset
}

// A FixtureRRset is an RRset as the fixture creates it in PowerDNS.
type FixtureRRset struct {
	// Name is the owner name, absolute, as PowerDNS's API wants it.
	Name, Type string
	TTL        uint32
	Records    []FixtureContent
}

// A FixtureContent is one record of an RRset.
type FixtureContent struct {
	Content  string
	Disabled bool
}

// DescribePowerDNSFixture returns the fixture with the ID id, without
// creating anything.
func DescribePowerDNSFixture(id string) *PowerDNSFixture {
	zone := id + ".nbpdns.example."
	ns := "ns1-" + id + ".nbpdns.example."
	hex := "ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789"
	one := func(name, typ string, ttl uint32, content string) FixtureRRset {
		return FixtureRRset{Name: name, Type: typ, TTL: ttl, Records: []FixtureContent{{Content: content}}}
	}
	long := strings.Repeat("a", 300)
	return &PowerDNSFixture{
		ID:         id,
		Zone:       zone,
		Others:     []string{id + "-b.nbpdns.example.", id + "-c.nbpdns.example."},
		Nameserver: ns,
		RRsets: []FixtureRRset{
			one(zone, "SOA", 3600, ns+" hostmaster."+zone+" 1 10800 3600 604800 3600"),
			one(zone, "NS", 3600, ns),
			{Name: "www." + zone, Type: "A", TTL: 300, Records: []FixtureContent{
				{Content: "192.0.2.10"}, {Content: "192.0.2.11"}, {Content: "192.0.2.14", Disabled: true},
			}},
			one("www."+zone, "AAAA", 3600, "2001:DB8::10"),
			{Name: "off." + zone, Type: "A", TTL: 3600, Records: []FixtureContent{{Content: "192.0.2.12", Disabled: true}}},
			one("alias."+zone, "CNAME", 3600, "www."+zone),
			// PowerDNS keeps a name's case as it's given.
			one(zone, "MX", 3600, "10 Mail."+strings.ToUpper(zone)),
			one("_sip._tcp."+zone, "SRV", 3600, "10 5 5060 sip."+zone),
			one(zone, "TXT", 3600, `"v=spf1 -all"`),
			one("quoted."+zone, "TXT", 3600, `"quoted" "two"`),
			one("long."+zone, "TXT", 3600, `"`+long[:255]+`" "`+long[255:]+`"`),
			one("xn--bcher-kva."+zone, "A", 3600, "192.0.2.13"),
			one("caa."+zone, "CAA", 3600, `0 issue "letsencrypt.org"`),
			one("_443._tcp.www."+zone, "TLSA", 3600, "3 1 1 "+strings.ToLower(hex)),
			one("www."+zone, "HTTPS", 3600, "1 . alpn=h3,h2 ipv4hint=192.0.2.10"),
			one(zone, "SSHFP", 3600, "4 2 "+hex),
			one("svc."+zone, "SVCB", 3600, "1 Web."+zone+" port=8443"),
			one("naptr."+zone, "NAPTR", 3600, `100 10 "S" "SIP+D2U" "" _sip._udp.`+zone),
		},
	}
}

// NewPowerDNSFixture creates a fixture with a random ID in p, and removes it
// when the test ends.
func NewPowerDNSFixture(t testing.TB, p PowerDNS) *PowerDNSFixture {
	t.Helper()
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return CreatePowerDNSFixture(t, p, "t"+hex.EncodeToString(b))
}

// CreatePowerDNSFixture creates the fixture with the ID id in p, and removes
// it when the test ends.
func CreatePowerDNSFixture(t testing.TB, p PowerDNS, id string) *PowerDNSFixture {
	t.Helper()
	f := DescribePowerDNSFixture(id)
	createPowerDNSZone(t, p, f.Zone, f.RRsets)
	for _, z := range f.Others {
		createPowerDNSZone(t, p, z, []FixtureRRset{
			{Name: z, Type: "SOA", TTL: 3600, Records: []FixtureContent{{Content: f.Nameserver + " hostmaster." + z + " 1 10800 3600 604800 3600"}}},
			{Name: z, Type: "NS", TTL: 3600, Records: []FixtureContent{{Content: f.Nameserver}}},
		})
	}
	return f
}

// createPowerDNSZone creates a native zone with sets as its RRsets in p, and
// deletes it when the test ends.
func createPowerDNSZone(t testing.TB, p PowerDNS, zone string, sets []FixtureRRset) {
	t.Helper()
	type record struct {
		Content  string `json:"content"`
		Disabled bool   `json:"disabled"`
	}
	type rrset struct {
		Name    string   `json:"name"`
		Type    string   `json:"type"`
		TTL     uint32   `json:"ttl"`
		Records []record `json:"records"`
	}
	rrsets := make([]rrset, len(sets))
	for i, s := range sets {
		rrsets[i] = rrset{Name: s.Name, Type: s.Type, TTL: s.TTL}
		for _, r := range s.Records {
			rrsets[i].Records = append(rrsets[i].Records, record(r))
		}
	}
	powerDNSDo(t, p, http.MethodPost, "zones", map[string]any{"name": zone, "kind": "Native", "nameservers": []string{}, "rrsets": rrsets})
	t.Cleanup(func() { powerDNSDo(t, p, http.MethodDelete, "zones/"+url.PathEscape(zone), nil) })
}

// powerDNSDo sends a request to path, under p's server, failing the test on
// any error.
func powerDNSDo(t testing.TB, p PowerDNS, method, path string, body any) {
	t.Helper()
	// A cleanup runs after the test's context is canceled.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Minute)
	defer cancel()
	var r io.Reader = http.NoBody
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.URL()+"/api/v1/servers/localhost/"+path, r)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-API-Key", PowerDNSAPIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v (is the lab up? make lab-up)", method, path, err)
	}
	defer resp.Body.Close()
	answer, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode/100 != 2 {
		t.Fatalf("%s %s on %s: %s: %s", method, path, p.Name, resp.Status, answer)
	}
}
