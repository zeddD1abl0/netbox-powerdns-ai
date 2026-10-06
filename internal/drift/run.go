package drift

import (
	"context"
	"maps"
	"slices"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
)

// NetBox reads NetBox's zones for the report.
type NetBox interface {
	// Zones lists the zones in views, or only the one named zone if it isn't
	// empty, without their RRsets.
	Zones(ctx context.Context, views []string, zone string) ([]dns.Zone, error)
	// Read reads zones, as Zones listed them, with their RRsets, and returns
	// them with the problems normalization found.
	Read(ctx context.Context, zones []dns.Zone) ([]dns.Zone, []dns.Problem, error)
}

// Primary reads a server group's primary for the report.
type Primary interface {
	// Zones lists the primary's zones, or only the one named zone if it
	// isn't empty, without their RRsets.
	Zones(ctx context.Context, zone string) ([]dns.Zone, error)
	// Read reads zones, as Zones listed them, with their RRsets, and returns
	// them with the problems normalization found.
	Read(ctx context.Context, zones []dns.Zone) ([]dns.Zone, []dns.Problem, error)
}

// A Group is a server group to compare: its configuration, and its primary,
// or the error that kept nbpdns from reaching it.
type Group struct {
	Config  config.Group
	Primary Primary
	Err     error
}

// Run compares every group, or only the zone named zone if it isn't empty.
// It reads NetBox once, for every group's views, then each group's primary
// in turn, and reads RRsets only for the zones it compares. If NetBox can't
// be read, Run returns the error. A group whose primary can't be read is
// marked failed, and the others are still compared (ADR-0027).
func Run(ctx context.Context, nb NetBox, groups []Group, zone string) (Report, error) {
	var views []string
	for _, g := range groups {
		for _, v := range g.Config.Views {
			if !slices.Contains(views, v) {
				views = append(views, v)
			}
		}
	}
	listed, err := nb.Zones(ctx, views, zone)
	if err != nil {
		return Report{}, err
	}

	// The NetBox zones some group compares, read once each.
	type id struct{ view, name string }
	needed := map[id]dns.Zone{}
	for _, g := range groups {
		for _, z := range inViews(listed, g.Config.Views) {
			if z.Active && g.Config.Policy(z.Name) != config.PolicyIgnore {
				needed[id{z.View, z.Name}] = z
			}
		}
	}
	read, nbProbs, err := nb.Read(ctx, slices.Collect(maps.Values(needed)))
	if err != nil {
		return Report{}, err
	}
	full := map[id]dns.Zone{}
	for _, z := range read {
		full[id{z.View, z.Name}] = z
	}
	withRRsets := func(zones []dns.Zone) []dns.Zone {
		out := make([]dns.Zone, len(zones))
		for i, z := range zones {
			out[i] = z
			if f, ok := full[id{z.View, z.Name}]; ok {
				out[i] = f
			}
		}
		return out
	}

	r := Report{Complete: true, Groups: []GroupReport{}}
	for _, g := range groups {
		gr := compareGroup(ctx, g, withRRsets(inViews(listed, g.Config.Views)), nbProbs, zone)
		if gr.Status != StatusOK {
			r.Complete = false
		}
		r.Drift = r.Drift || gr.Counts.Drifted()
		r.Groups = append(r.Groups, gr)
	}
	return r, nil
}

// compareGroup reads g's primary and compares it with nb, the NetBox zones of
// g's views.
func compareGroup(ctx context.Context, g Group, nb []dns.Zone, nbProbs []dns.Problem, zone string) GroupReport {
	failed := func(err error) GroupReport {
		return GroupReport{Group: g.Config.Name, Status: StatusFailed, Error: err.Error(), Zones: []ZoneReport{},
			Unmanaged: []string{}, Problems: []dns.Problem{}, Warnings: []string{}}
	}
	if g.Err != nil {
		return failed(g.Err)
	}
	listed, err := g.Primary.Zones(ctx, zone)
	if err != nil {
		return failed(err)
	}
	compared := map[string]bool{}
	for _, z := range nb {
		if z.Active && g.Config.Policy(z.Name) != config.PolicyIgnore {
			compared[z.Name] = true
		}
	}
	var toRead, rest []dns.Zone
	for _, z := range listed {
		if compared[z.Name] {
			toRead = append(toRead, z)
		} else {
			rest = append(rest, z)
		}
	}
	read, pdProbs, err := g.Primary.Read(ctx, toRead)
	if err != nil {
		return failed(err)
	}
	return Compare(g.Config, nb, append(read, rest...), append(slices.Clone(nbProbs), pdProbs...))
}

// inViews returns the zones in views.
func inViews(zones []dns.Zone, views []string) []dns.Zone {
	var out []dns.Zone
	for _, z := range zones {
		if slices.Contains(views, z.View) {
			out = append(out, z)
		}
	}
	return out
}
