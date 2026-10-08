// Package drift compares the zones NetBox assigns to each PowerDNS server
// group with what the group's primary serves, and reports the differences
// (ADR-0027). It only reads. Compare is pure; Run reads both sides through
// the NetBox and Primary interfaces, the backend interfaces of Q-039.
package drift

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
)

// A zone's state in the report.
const (
	// StateInSync is a zone that the primary serves as NetBox says.
	StateInSync = "in_sync"
	// StateDrift is a zone whose RRsets differ.
	StateDrift = "drift"
	// StateMissing is an active NetBox zone that the primary doesn't have.
	StateMissing = "missing"
	// StateInactive is a NetBox zone that isn't active, which the primary
	// serves.
	StateInactive = "inactive_in_netbox"
	// StateIgnored is a zone whose policy is ignore, which isn't compared.
	StateIgnored = "ignored"
	// StateUnmanaged names the count of a primary's zones that NetBox
	// doesn't assign to its group. They're listed apart from the zones, so
	// no ZoneReport has this state.
	StateUnmanaged = "unmanaged"
)

// States are every state that Counts counts, as the metrics and the status
// page name them.
var States = []string{StateInSync, StateDrift, StateMissing, StateInactive, StateIgnored, StateUnmanaged}

// DriftedStates are the states of a zone that drifted.
var DriftedStates = []string{StateDrift, StateMissing, StateInactive}

// IsDrifted reports whether a zone in state drifted.
func IsDrifted(state string) bool { return slices.Contains(DriftedStates, state) }

// How an RRset differs.
const (
	// ChangeMissing is an RRset that NetBox has and the primary doesn't serve.
	ChangeMissing = "missing"
	// ChangeExtra is an RRset that the primary serves and NetBox doesn't have.
	ChangeExtra = "extra"
	// ChangeChanged is an RRset whose values or TTL differ.
	ChangeChanged = "changed"
)

// ChangeKinds are every kind of change.
var ChangeKinds = []string{ChangeMissing, ChangeExtra, ChangeChanged}

// A group's status in the report.
const (
	// StatusOK is a group that was compared.
	StatusOK = "ok"
	// StatusFailed is a group whose primary couldn't be read.
	StatusFailed = "failed"
)

// A Side is an RRset as one side serves it.
type Side struct {
	TTL    uint32   `json:"ttl"`
	Values []string `json:"values"`
}

// A Change is an RRset that differs between NetBox and the primary.
type Change struct {
	Name string `json:"name"`
	Type string `json:"type"`
	// Kind is ChangeMissing, ChangeExtra or ChangeChanged.
	Kind     string `json:"kind"`
	NetBox   *Side  `json:"netbox,omitempty"`
	PowerDNS *Side  `json:"powerdns,omitempty"`
}

// A ZoneReport is one zone's comparison.
type ZoneReport struct {
	Zone string `json:"zone"`
	// View is the zone's NetBox view, if NetBox has the zone.
	View   string `json:"view,omitempty"`
	Policy string `json:"policy"`
	// State is one of the State constants.
	State string `json:"state"`
	// The SOA serials, which aren't compared.
	NetBoxSerial   uint32   `json:"netbox_serial,omitempty"`
	PowerDNSSerial uint32   `json:"powerdns_serial,omitempty"`
	Changes        []Change `json:"changes"`
}

// Counts counts a group's zones by state.
type Counts struct {
	InSync    int `json:"in_sync"`
	Drift     int `json:"drift"`
	Missing   int `json:"missing"`
	Inactive  int `json:"inactive_in_netbox"`
	Ignored   int `json:"ignored"`
	Unmanaged int `json:"unmanaged"`
}

// DriftedZones counts the zones that drifted: those in drift, missing, or
// served though inactive in NetBox.
func (c Counts) DriftedZones() int { return c.Drift + c.Missing + c.Inactive }

// ByState returns the counts by state, as States names them.
func (c Counts) ByState() map[string]int {
	return map[string]int{
		StateInSync: c.InSync, StateDrift: c.Drift, StateMissing: c.Missing,
		StateInactive: c.Inactive, StateIgnored: c.Ignored, StateUnmanaged: c.Unmanaged,
	}
}

// Drifted reports whether any zone counted drifted.
func (c Counts) Drifted() bool { return c.DriftedZones() > 0 }

// A GroupReport is one server group's comparison.
type GroupReport struct {
	Group string `json:"group"`
	// Status is StatusOK, or StatusFailed with Error saying why.
	Status string       `json:"status"`
	Error  string       `json:"error,omitempty"`
	Zones  []ZoneReport `json:"zones"`
	// Unmanaged are the zones on the primary that NetBox doesn't assign to
	// the group. They aren't drift.
	Unmanaged []string `json:"unmanaged"`
	// Problems are what normalization worked around, on either side.
	Problems []dns.Problem `json:"problems"`
	// Warnings are problems with the configuration or NetBox's data that
	// the report worked around.
	Warnings []string `json:"warnings"`
	Counts   Counts   `json:"counts"`
}

// A Report is the comparison of every group.
type Report struct {
	// Complete reports whether every group was read and compared.
	Complete bool `json:"complete"`
	// Drift reports whether any group's zones drifted.
	Drift  bool          `json:"drift"`
	Groups []GroupReport `json:"groups"`
	// NetBox, with Options.ReadNetBox, is every NetBox zone in the groups'
	// views, sorted by view and name: each active one with its RRsets, each
	// inactive one without. It's never nil then, unless NetBoxErr says why
	// the zones that aren't compared couldn't be read. Neither is part of
	// the report's JSON.
	NetBox    []dns.Zone `json:"-"`
	NetBoxErr error      `json:"-"`
}

// Compare compares group g's NetBox zones, nb, which are the zones of its
// views, with the primary's zones, pd. Zones that are compared must carry
// their RRsets; the others needn't. probs are the problems normalization
// found in either side's zones; Compare keeps the ones of g's zones.
func Compare(g config.Group, nb, pd []dns.Zone, probs []dns.Problem) GroupReport {
	r := GroupReport{Group: g.Name, Status: StatusOK, Zones: []ZoneReport{}, Unmanaged: []string{},
		Problems: []dns.Problem{}, Warnings: []string{}}
	primary := map[string]dns.Zone{}
	for _, z := range pd {
		primary[z.Name] = z
	}
	netbox := map[string]dns.Zone{}
	for _, z := range sortByView(nb) {
		if first, ok := netbox[z.Name]; ok {
			r.Warnings = append(r.Warnings, fmt.Sprintf("zone %s is in the views %s and %s, which one server can't both serve; "+
				"the one in %s is compared", z.Name, first.View, z.View, first.View))
			continue
		}
		netbox[z.Name] = z
	}

	for _, name := range slices.SortedFunc(maps.Keys(netbox), dns.CompareNames) {
		z := netbox[name]
		zr := ZoneReport{Zone: name, View: z.View, Policy: g.Policy(name), NetBoxSerial: z.SOASerial, Changes: []Change{}}
		p, onPrimary := primary[name]
		if onPrimary {
			zr.PowerDNSSerial = p.SOASerial
		}
		switch {
		case zr.Policy == config.PolicyIgnore:
			zr.State = StateIgnored
		case !z.Active && onPrimary:
			zr.State = StateInactive
		case !z.Active:
			// Expected absent, and absent.
			continue
		case !onPrimary:
			zr.State = StateMissing
		default:
			zr.Changes = compareRRsets(z, p)
			zr.State = StateInSync
			if len(zr.Changes) > 0 {
				zr.State = StateDrift
			}
		}
		r.Zones = append(r.Zones, zr)
	}
	for _, name := range slices.SortedFunc(maps.Keys(primary), dns.CompareNames) {
		if _, ok := netbox[name]; !ok {
			r.Unmanaged = append(r.Unmanaged, name)
		}
	}
	for _, zone := range slices.Sorted(maps.Keys(g.ZonePolicies)) {
		if _, ok := netbox[zone]; !ok {
			r.Warnings = append(r.Warnings, fmt.Sprintf("zone_policies names %s, which isn't in any of the group's NetBox views", zone))
		}
	}
	// A NetBox problem is the group's only if it's in the zone compared, of
	// that view. The primary's problems have no view, and are the group's.
	for _, p := range probs {
		if z, ok := netbox[p.Zone]; ok && (p.View == "" || p.View == z.View) {
			r.Problems = append(r.Problems, p)
		}
	}
	r.Counts = count(r)
	return r
}

// sortByView returns zones sorted by view, so the first of two zones that
// share a name is the same every run.
func sortByView(zones []dns.Zone) []dns.Zone {
	out := slices.Clone(zones)
	slices.SortStableFunc(out, func(a, b dns.Zone) int { return strings.Compare(a.View, b.View) })
	return out
}

func count(r GroupReport) Counts {
	c := Counts{Unmanaged: len(r.Unmanaged)}
	for _, z := range r.Zones {
		switch z.State {
		case StateInSync:
			c.InSync++
		case StateDrift:
			c.Drift++
		case StateMissing:
			c.Missing++
		case StateInactive:
			c.Inactive++
		case StateIgnored:
			c.Ignored++
		}
	}
	return c
}

// compareRRsets returns how the RRsets that nb and pd serve differ.
func compareRRsets(nb, pd dns.Zone) []Change {
	a, b := served(nb), served(pd)
	keys := slices.Collect(maps.Keys(a))
	for k := range b {
		if _, ok := a[k]; !ok {
			keys = append(keys, k)
		}
	}
	slices.SortFunc(keys, func(x, y key) int { return dns.CompareRRsets(x.name, x.typ, y.name, y.typ) })
	changes := []Change{}
	for _, k := range keys {
		sa, inA := a[k]
		sb, inB := b[k]
		c := Change{Name: k.name, Type: k.typ}
		switch {
		case !inB:
			c.Kind, c.NetBox = ChangeMissing, &sa
		case !inA:
			c.Kind, c.PowerDNS = ChangeExtra, &sb
		case sa.TTL != sb.TTL || !slices.Equal(comparedValues(k.typ, sa.Values), comparedValues(k.typ, sb.Values)):
			c.Kind, c.NetBox, c.PowerDNS = ChangeChanged, &sa, &sb
		default:
			continue
		}
		changes = append(changes, c)
	}
	return changes
}

// A key names an RRset.
type key struct{ name, typ string }

// served returns the RRsets of z that it serves: their active records, with
// the RRset's TTL. An RRset with no active record isn't served.
func served(z dns.Zone) map[key]Side {
	out := map[key]Side{}
	for _, s := range z.RRsets {
		var values []string
		for _, r := range s.Records {
			if r.Active {
				values = append(values, r.Value)
			}
		}
		if len(values) > 0 {
			slices.Sort(values)
			out[key{s.Name, s.Type}] = Side{TTL: s.TTL, Values: slices.Compact(values)}
		}
	}
	return out
}

// comparedValues returns an RRset's values as they're compared: an SOA's without
// its serial, which PowerDNS rewrites itself (ADR-0027), and any other type's
// as they are.
func comparedValues(typ string, values []string) []string {
	if typ != "SOA" {
		return values
	}
	out := make([]string, len(values))
	for i, v := range values {
		f := strings.Fields(v)
		if len(f) == 7 {
			f[2] = ""
		}
		out[i] = strings.Join(f, " ")
	}
	return out
}
