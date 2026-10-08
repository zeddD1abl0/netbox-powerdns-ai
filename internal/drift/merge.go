package drift

import (
	"slices"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
)

// Merge returns a group's last report, old, with r, a zone refresh's report
// of the group, merged in (ADR-0036). For each name r compared, its zone,
// whether unmanaged, its problems and its warnings are r's; a zone that
// neither side has any more leaves the report. The counts are counted
// again. Neither report is changed: the result's slices are its own, so a
// report that something else holds stays as it was.
func Merge(old, r GroupReport) GroupReport {
	compared := map[string]bool{}
	for _, n := range r.Compared {
		compared[n] = true
	}
	out := GroupReport{
		Group: old.Group, Status: StatusOK,
		Zones:     replace(old.Zones, r.Zones, func(z ZoneReport) bool { return compared[z.Zone] }),
		Unmanaged: replace(old.Unmanaged, r.Unmanaged, func(name string) bool { return compared[name] }),
		Problems:  replace(old.Problems, r.Problems, func(p dns.Problem) bool { return compared[p.Zone] }),
		Warnings:  replace(old.Warnings, r.Warnings, func(w Warning) bool { return compared[w.Zone] }),
	}
	slices.SortFunc(out.Zones, func(a, b ZoneReport) int { return dns.CompareNames(a.Zone, b.Zone) })
	slices.SortFunc(out.Unmanaged, dns.CompareNames)
	out.Counts = count(out)
	return out
}

// replace returns a new slice of the items of old that aren't stale,
// followed by those of fresh.
func replace[T any](old, fresh []T, stale func(T) bool) []T {
	out := make([]T, 0, len(old)+len(fresh))
	for _, v := range old {
		if !stale(v) {
			out = append(out, v)
		}
	}
	return append(out, fresh...)
}
