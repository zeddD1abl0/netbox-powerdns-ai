package lab

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
)

// A DriftFixture is a set of zones created in a lab NetBox and on a lab
// PowerDNS server for one test of the drift report, with one of each case
// of ADR-0027. NetBox's zones are all in View. Every name in it contains its
// ID, so fixtures of concurrent tests don't collide.
type DriftFixture struct {
	ID   string
	View string
	// Nameserver is every zone's name server, outside the zones.
	Nameserver string
	// The zones, by the case each holds. Names are absolute. Every zone on
	// both sides has the same SOA, apart from its serial, unless it says
	// otherwise, and the same NS records.
	//
	//   - InSync has the same RRsets on both sides, apart from records that
	//     neither side serves: in NetBox, inactive records, and on the
	//     primary, records turned off, with other values.
	//   - Drift has one RRset of each change. gone A is only in NetBox, and
	//     stray TXT only on the primary. www A's value differs, and mail A's
	//     TTL. The SOA's contact differs.
	//   - Missing is active in NetBox, and not on the primary.
	//   - Parked is parked in NetBox, and on the primary.
	//   - Ignored has a www A that differs. A test gives it the policy ignore.
	//   - Unmanaged is only on the primary.
	InSync, Drift, Missing, Parked, Ignored, Unmanaged string
	// ReaderToken is the v2 token of a user who can only view the DNS
	// plugin's objects.
	ReaderToken string
}

// DescribeDriftFixture returns the drift fixture with the ID id, without its
// token, and without creating anything.
func DescribeDriftFixture(id string) *DriftFixture {
	zone := func(name string) string { return id + "-" + name + ".nbpdns.example." }
	return &DriftFixture{
		ID:         id,
		View:       "nbpdns-" + id,
		Nameserver: "ns1-" + id + ".nbpdns.example.",
		InSync:     zone("sync"),
		Drift:      zone("drift"),
		Missing:    zone("missing"),
		Parked:     zone("parked"),
		Ignored:    zone("ignored"),
		Unmanaged:  zone("unmanaged"),
	}
}

// NewDriftFixture creates a drift fixture with a random ID in nb and p, and
// removes it when the test ends.
func NewDriftFixture(t testing.TB, nb NetBox, p PowerDNS) *DriftFixture {
	t.Helper()
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return CreateDriftFixture(t, nb, p, "t"+hex.EncodeToString(b))
}

// CreateDriftFixture creates the drift fixture with the ID id in nb and p,
// and removes it when the test ends.
func CreateDriftFixture(t testing.TB, nb NetBox, p PowerDNS, id string) *DriftFixture {
	t.Helper()
	f := DescribeDriftFixture(id)
	a := &adminClient{t: t, nb: nb}
	ns := a.create("plugins/netbox-dns/nameservers/", map[string]any{"name": strings.TrimSuffix(f.Nameserver, ".")})
	view := a.create("plugins/netbox-dns/views/", map[string]any{"name": f.View})

	// The SOA's fields, apart from its names and serial, are set on both
	// sides, so that they don't depend on the plugin's defaults.
	const soaTimers = "10800 3600 604800 3600"
	inNetBox := func(zone, status string, records ...FixtureRecord) {
		zoneID := a.create("plugins/netbox-dns/zones/", map[string]any{
			"name": strings.TrimSuffix(zone, "."), "view": view, "status": status, "default_ttl": 3600,
			"nameservers": []int{ns}, "soa_mname": ns, "soa_rname": "hostmaster." + zone,
			"soa_ttl": 3600, "soa_refresh": 10800, "soa_retry": 3600, "soa_expire": 604800, "soa_minimum": 3600,
		})
		a.records(zoneID, records)
	}
	onPrimary := func(zone, contact string, sets ...FixtureRRset) {
		createPowerDNSZone(t, p, zone, append([]FixtureRRset{
			{Name: zone, Type: "SOA", TTL: 3600, Records: []FixtureContent{{Content: f.Nameserver + " " + contact + " 1 " + soaTimers}}},
			{Name: zone, Type: "NS", TTL: 3600, Records: []FixtureContent{{Content: f.Nameserver}}},
		}, sets...))
	}
	one := func(name, typ string, ttl uint32, content string) FixtureRRset {
		return FixtureRRset{Name: name, Type: typ, TTL: ttl, Records: []FixtureContent{{Content: content}}}
	}

	z := f.InSync
	inNetBox(z, "active",
		FixtureRecord{Name: "www", Type: "A", Value: "192.0.2.10"},
		FixtureRecord{Name: "www", Type: "A", Value: "192.0.2.14", Status: "inactive"},
		FixtureRecord{Name: "off", Type: "A", Value: "192.0.2.12", Status: "inactive"},
		FixtureRecord{Name: "@", Type: "MX", Value: "10 mail"})
	onPrimary(z, "hostmaster."+z,
		FixtureRRset{Name: "www." + z, Type: "A", TTL: 3600, Records: []FixtureContent{
			{Content: "192.0.2.10"}, {Content: "192.0.2.15", Disabled: true},
		}},
		FixtureRRset{Name: "off." + z, Type: "A", TTL: 3600, Records: []FixtureContent{{Content: "192.0.2.13", Disabled: true}}},
		one(z, "MX", 3600, "10 mail."+z))

	z = f.Drift
	inNetBox(z, "active",
		FixtureRecord{Name: "www", Type: "A", Value: "192.0.2.10"},
		FixtureRecord{Name: "mail", Type: "A", Value: "192.0.2.25", TTL: 300},
		FixtureRecord{Name: "gone", Type: "A", Value: "192.0.2.20"})
	onPrimary(z, "admin."+z,
		one("www."+z, "A", 3600, "192.0.2.99"),
		one("mail."+z, "A", 600, "192.0.2.25"),
		one("stray."+z, "TXT", 3600, `"stray"`))

	inNetBox(f.Missing, "active")

	inNetBox(f.Parked, "parked")
	onPrimary(f.Parked, "hostmaster."+f.Parked)

	z = f.Ignored
	inNetBox(z, "active", FixtureRecord{Name: "www", Type: "A", Value: "192.0.2.10"})
	onPrimary(z, "hostmaster."+z, one("www."+z, "A", 3600, "192.0.2.99"))

	onPrimary(f.Unmanaged, "hostmaster."+f.Unmanaged)

	f.ReaderToken = a.user("nbpdns-drift-"+id, true)
	return f
}
