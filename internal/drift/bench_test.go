package drift

import (
	"fmt"
	"testing"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
)

// scaleZones returns the scale target of REQ-043 on each side: 1,000 zones
// of 100 records, the primary's with one record in a hundred changed.
func scaleZones() (nb, pd []dns.Zone) {
	for i := range 1000 {
		name := fmt.Sprintf("zone-%04d.example.", i)
		var a, b []dns.RRset
		for j := range 99 {
			owner := fmt.Sprintf("host-%02d.%s", j, name)
			value := fmt.Sprintf("10.%d.%d.%d", i/250, i%250, j)
			a = append(a, rrset(owner, "A", 3600, rec(value)))
			if (i*99+j)%100 == 0 {
				value = "192.0.2.1"
			}
			b = append(b, rrset(owner, "A", 3600, rec(value)))
		}
		nb = append(nb, zone(name, "v", 1, a...))
		pd = append(pd, zone(name, "", 2, b...))
	}
	return nb, pd
}

// BenchmarkCompareAtScale compares the scale target of REQ-043: 1,000 zones
// and 100,000 records on each side. It must take under a second.
func BenchmarkCompareAtScale(b *testing.B) {
	nb, pd := scaleZones()
	g := config.Group{Name: "site-a", Views: []string{"v"}, DriftPolicy: config.PolicyReport}
	b.ReportAllocs()
	for b.Loop() {
		r := Compare(g, nb, pd, nil)
		if r.Counts.Drift == 0 {
			b.Fatal("no drift found")
		}
	}
}

func TestScaleZones(t *testing.T) {
	nb, pd := scaleZones()
	records := 0
	for _, z := range nb {
		for _, s := range z.RRsets {
			records += len(s.Records)
		}
	}
	r := Compare(config.Group{Name: "site-a", Views: []string{"v"}, DriftPolicy: config.PolicyReport}, nb, pd, nil)
	changes := 0
	for _, z := range r.Zones {
		changes += len(z.Changes)
	}
	if len(nb) != 1000 || len(pd) != 1000 || records != 100000 || changes != 990 {
		t.Errorf("%d and %d zones, %d records, %d changes", len(nb), len(pd), records, changes)
	}
}
