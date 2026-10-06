package drift

import (
	"context"
	"fmt"
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

// ZoneNotFoundError is Run's error for a zone that's in none of the groups'
// NetBox views, and on none of their primaries.
type ZoneNotFoundError struct{ Zone string }

func (e *ZoneNotFoundError) Error() string {
	return fmt.Sprintf("zone %s isn't in any server group's NetBox views, or on any group's primary", e.Zone)
}

// Run compares every group, or only the zone named zone if it isn't empty.
// It lists NetBox's zones once, for every group's views, then each group's
// primary's zones in turn. It then reads RRsets, from NetBox once and from
// each primary, only for the zones it compares, and compares each group. If
// NetBox can't be read, Run returns the error. A group whose primary can't
// be read is marked failed, and the others are still compared (ADR-0027).
// If zone isn't empty and every group was read, but neither side has the
// zone, Run returns a ZoneNotFoundError, so that a mistyped name isn't
// reported as in sync.
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

	// List each primary first: a group that can't be listed needs nothing
	// from NetBox, and a zone that its primary doesn't have needs no RRsets.
	type id struct{ view, name string }
	type state struct {
		cfg    config.Group
		nb, pd []dns.Zone
		err    error
	}
	states := make([]state, len(groups))
	needed := map[id]dns.Zone{}
	for i, g := range groups {
		st := &states[i]
		st.cfg, st.nb, st.err = forZone(g.Config, zone), inViews(listed, g.Config.Views), g.Err
		if st.err == nil {
			st.pd, st.err = g.Primary.Zones(ctx, zone)
		}
		if st.err != nil {
			continue
		}
		onPrimary := map[string]bool{}
		for _, z := range st.pd {
			onPrimary[z.Name] = true
		}
		for _, z := range st.nb {
			if compared(st.cfg, z) && onPrimary[z.Name] {
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

	r := Report{Complete: true, Groups: []GroupReport{}}
	for i, g := range groups {
		st := states[i]
		var gr GroupReport
		if st.err != nil {
			gr = failed(g.Config.Name, st.err)
		} else {
			nbZones := make([]dns.Zone, len(st.nb))
			for j, z := range st.nb {
				nbZones[j] = z
				if f, ok := full[id{z.View, z.Name}]; ok {
					nbZones[j] = f
				}
			}
			gr = compareGroup(ctx, g.Primary, st.cfg, nbZones, st.pd, nbProbs)
		}
		if gr.Status != StatusOK {
			r.Complete = false
		}
		r.Drift = r.Drift || gr.Counts.Drifted()
		r.Groups = append(r.Groups, gr)
	}
	if zone != "" && r.Complete && len(listed) == 0 {
		// A zone on a primary that NetBox doesn't have is unmanaged.
		found := false
		for _, g := range r.Groups {
			found = found || len(g.Unmanaged) > 0
		}
		if !found {
			return r, &ZoneNotFoundError{Zone: zone}
		}
	}
	return r, nil
}

// forZone returns g as Run compares it: with only zone's policy, if zone
// isn't empty. Then only that zone is listed, so the other zones' policies
// can't be checked against NetBox's.
func forZone(g config.Group, zone string) config.Group {
	if zone == "" {
		return g
	}
	policies := map[string]string{}
	if p, ok := g.ZonePolicies[zone]; ok {
		policies[zone] = p
	}
	g.ZonePolicies = policies
	return g
}

// compared reports whether g compares the NetBox zone z, if its primary has
// it: z is active, and its policy isn't ignore.
func compared(g config.Group, z dns.Zone) bool {
	return z.Active && g.Policy(z.Name) != config.PolicyIgnore
}

// failed returns the report of group, which couldn't be read.
func failed(group string, err error) GroupReport {
	return GroupReport{Group: group, Status: StatusFailed, Error: err.Error(), Zones: []ZoneReport{},
		Unmanaged: []string{}, Problems: []dns.Problem{}, Warnings: []string{}}
}

// compareGroup reads the RRsets of the zones that group g compares from its
// primary, p, which listed pd, and compares them with nb, the NetBox zones
// of g's views.
func compareGroup(ctx context.Context, p Primary, g config.Group, nb, pd []dns.Zone, nbProbs []dns.Problem) GroupReport {
	want := map[string]bool{}
	for _, z := range nb {
		if compared(g, z) {
			want[z.Name] = true
		}
	}
	var toRead, rest []dns.Zone
	for _, z := range pd {
		if want[z.Name] {
			toRead = append(toRead, z)
		} else {
			rest = append(rest, z)
		}
	}
	read, pdProbs, err := p.Read(ctx, toRead)
	if err != nil {
		return failed(g.Name, err)
	}
	return Compare(g, nb, append(read, rest...), append(slices.Clone(nbProbs), pdProbs...))
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
