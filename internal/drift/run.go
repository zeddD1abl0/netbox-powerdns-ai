package drift

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
)

// NetBox reads NetBox's zones for the report.
type NetBox interface {
	// Zones lists the zones in views, or only those with names, absolute
	// names, if names isn't nil, without their RRsets.
	Zones(ctx context.Context, views, names []string) ([]dns.Zone, error)
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

// A ZoneRef names a zone in a NetBox view.
type ZoneRef struct {
	View string
	// Name is the zone's absolute name.
	Name string
}

// Options say what Run compares, and how.
type Options struct {
	// Zone, if it isn't empty, is the one zone compared, as an absolute
	// name.
	Zone string
	// Zones, if it isn't nil, makes Run a zone refresh (ADR-0036): each
	// group compares only the zones of Zones in its views, by name, in each
	// of its views, as a full comparison would compare them. A group with
	// none isn't compared, and isn't in the report. Unlike Zone, a zone
	// that neither side has isn't an error: it was deleted.
	Zones []ZoneRef
	// Concurrency is how many groups are listed, read and compared at once.
	// Below 1, it's 1.
	Concurrency int
	// ReadNetBox reads the RRsets of every active NetBox zone in the
	// groups' views, not only of those compared, and returns every zone
	// listed as the report's NetBox zones, for nbpdns serve's API
	// (ADR-0033). The extra reads change nothing in the comparison: they're
	// made apart from it, their problems are left out of it, and their
	// failure is the report's NetBoxErr, not Run's error.
	ReadNetBox bool
}

// A Scope is what a zone refresh compared: the zones with Names, absolute
// names, in Views.
type Scope struct {
	Views []string
	Names []string
}

// Run compares every group, or only the zones o names. It lists NetBox's
// zones once, for every group's views, then each group's primary's zones.
// It then reads RRsets, from NetBox once and from each primary, only for the
// zones it compares, and compares each group. Up to o.Concurrency groups are
// listed, read and compared at once, and the report keeps their order. If
// NetBox can't be read, Run returns the error. A group whose primary can't
// be read is marked failed, and the others are still compared (ADR-0027).
// If o names a zone, and every group was read, but neither side has it, Run
// returns a ZoneNotFoundError, so that a mistyped name isn't reported as in
// sync.
func Run(ctx context.Context, nb NetBox, groups []Group, o Options) (Report, error) {
	zone := o.Zone
	// The names each group compares: nil for every zone.
	names := make([][]string, len(groups))
	switch {
	case o.Zones != nil:
		var kept []Group
		var keptNames [][]string
		for _, g := range groups {
			if n := namesIn(o.Zones, g.Config.Views); len(n) > 0 {
				kept, keptNames = append(kept, g), append(keptNames, n)
			}
		}
		groups, names = kept, keptNames
	case zone != "":
		for i := range names {
			names[i] = []string{zone}
		}
	}
	var views, allNames []string
	for i, g := range groups {
		for _, v := range g.Config.Views {
			if !slices.Contains(views, v) {
				views = append(views, v)
			}
		}
		for _, n := range names[i] {
			if !slices.Contains(allNames, n) {
				allNames = append(allNames, n)
			}
		}
	}
	if o.Zones != nil && len(groups) == 0 {
		// No group serves them, so there's nothing to read.
		return Report{Complete: true, Groups: []GroupReport{}, NetBox: []dns.Zone{}, Scope: &Scope{}}, nil
	}
	listed, err := nb.Zones(ctx, views, allNames)
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
	each(len(groups), o.Concurrency, func(i int) {
		g, st := groups[i], &states[i]
		st.cfg, st.nb, st.err = forZones(g.Config, names[i]), named(inViews(listed, g.Config.Views), names[i]), g.Err
		if st.err == nil {
			st.pd, st.err = listPrimary(ctx, g.Primary, names[i])
		}
	})
	needed := map[id]dns.Zone{}
	for _, st := range states {
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

	reports := make([]GroupReport, len(groups))
	each(len(groups), o.Concurrency, func(i int) {
		g, st := groups[i], states[i]
		if st.err != nil {
			reports[i] = failed(g.Config.Name, st.err)
			return
		}
		nbZones := make([]dns.Zone, len(st.nb))
		for j, z := range st.nb {
			nbZones[j] = z
			if f, ok := full[id{z.View, z.Name}]; ok {
				nbZones[j] = f
			}
		}
		reports[i] = compareGroup(ctx, g.Primary, st.cfg, nbZones, st.pd, nbProbs)
	})
	r := Report{Complete: true, Groups: []GroupReport{}}
	if o.Zones != nil {
		r.Scope = &Scope{Views: views, Names: allNames}
		for i := range reports {
			reports[i].Compared = names[i]
		}
	}
	if o.ReadNetBox {
		r.NetBox, r.NetBoxErr = readRest(ctx, nb, listed, read, func(z dns.Zone) bool {
			_, ok := needed[id{z.View, z.Name}]
			return ok
		})
	}
	for _, gr := range reports {
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

// readRest returns every zone listed: those read, the other active zones,
// which it reads, and the inactive ones, bare, sorted by view and name. It
// reads the others in a call of their own, so that their failure, which it
// returns, can't fail the comparison. Their problems aren't the
// comparison's, so they're dropped. The zones are never nil.
func readRest(ctx context.Context, nb NetBox, listed, read []dns.Zone, wasRead func(dns.Zone) bool) ([]dns.Zone, error) {
	out := make([]dns.Zone, 0, len(listed))
	out = append(out, read...)
	var rest []dns.Zone
	for _, z := range listed {
		switch {
		case wasRead(z):
		case z.Active:
			rest = append(rest, z)
		default:
			out = append(out, z)
		}
	}
	if len(rest) > 0 {
		more, _, err := nb.Read(ctx, rest)
		if err != nil {
			return nil, err
		}
		out = append(out, more...)
	}
	slices.SortFunc(out, func(a, b dns.Zone) int {
		return cmp.Or(strings.Compare(a.View, b.View), dns.CompareNames(a.Name, b.Name))
	})
	return out, nil
}

// each calls fn for 0 to n-1, running up to limit at once, and returns when
// every call has.
func each(n, limit int, fn func(i int)) {
	sem := make(chan struct{}, max(limit, 1))
	var wg sync.WaitGroup
	for i := range n {
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			fn(i)
		})
	}
	wg.Wait()
}

// forZones returns g as Run compares it: with only the policies of the
// zones named names, unless names is nil. Then only those zones are listed,
// so the other zones' policies can't be checked against NetBox's.
func forZones(g config.Group, names []string) config.Group {
	if names == nil {
		return g
	}
	policies := map[string]string{}
	for _, n := range names {
		if p, ok := g.ZonePolicies[n]; ok {
			policies[n] = p
		}
	}
	g.ZonePolicies = policies
	return g
}

// namesIn returns the names of zones in views, each once, in order.
func namesIn(zones []ZoneRef, views []string) []string {
	var out []string
	for _, z := range zones {
		if slices.Contains(views, z.View) && !slices.Contains(out, z.Name) {
			out = append(out, z.Name)
		}
	}
	return out
}

// named returns the zones named names, or every zone if names is nil.
func named(zones []dns.Zone, names []string) []dns.Zone {
	if names == nil {
		return zones
	}
	var out []dns.Zone
	for _, z := range zones {
		if slices.Contains(names, z.Name) {
			out = append(out, z)
		}
	}
	return out
}

// listPrimary lists p's zones named names, or every zone if names is nil.
// One zone is looked up; any more are filtered from the list of every zone,
// which is one request.
func listPrimary(ctx context.Context, p Primary, names []string) ([]dns.Zone, error) {
	if len(names) == 1 {
		return p.Zones(ctx, names[0])
	}
	zones, err := p.Zones(ctx, "")
	if err != nil {
		return nil, err
	}
	return named(zones, names), nil
}

// compared reports whether g compares the NetBox zone z, if its primary has
// it: z is active, and its policy isn't ignore.
func compared(g config.Group, z dns.Zone) bool {
	return z.Active && g.Policy(z.Name) != config.PolicyIgnore
}

// failed returns the report of group, which couldn't be read.
func failed(group string, err error) GroupReport {
	return GroupReport{Group: group, Status: StatusFailed, Error: err.Error(), Zones: []ZoneReport{},
		Unmanaged: []string{}, Problems: []dns.Problem{}, Warnings: []Warning{}}
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
