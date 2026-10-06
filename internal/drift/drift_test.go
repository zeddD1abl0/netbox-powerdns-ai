package drift

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
)

const soa = "ns1.example.com. hostmaster.example.com. %d 10800 3600 604800 3600"

// rec returns an active record.
func rec(value string) dns.Record { return dns.Record{Value: value, Status: "active", Active: true} }

// off returns a record that isn't served.
func off(value string) dns.Record { return dns.Record{Value: value, Status: "inactive"} }

func rrset(name, typ string, ttl uint32, records ...dns.Record) dns.RRset {
	return dns.RRset{Name: name, Type: typ, TTL: ttl, Records: records}
}

// zone returns an active zone in view with the SOA serial serial and rrsets.
func zone(name, view string, serial uint32, rrsets ...dns.RRset) dns.Zone {
	all := append([]dns.RRset{rrset(name, "SOA", 3600, rec(fmt.Sprintf(soa, serial)))}, rrsets...)
	return dns.Zone{Name: name, View: view, Active: true, SOASerial: serial, RRsets: all}
}

func TestCompare(t *testing.T) {
	const z = "example.com."
	www := func(ttl uint32, values ...string) dns.RRset {
		var rs []dns.Record
		for _, v := range values {
			rs = append(rs, rec(v))
		}
		return rrset("www."+z, "A", ttl, rs...)
	}
	parked := zone(z, "v", 1)
	parked.Active = false
	tests := []struct {
		name        string
		nb, pd      []dns.Zone
		policy      string
		wantState   string
		wantChanges []Change
	}{
		{"in sync, whatever the serials", []dns.Zone{zone(z, "v", 1, www(300, "192.0.2.1"))},
			[]dns.Zone{zone(z, "", 2026100601, www(300, "192.0.2.1"))}, "", StateInSync, []Change{}},
		{"a missing RRset", []dns.Zone{zone(z, "v", 1, www(300, "192.0.2.1"))}, []dns.Zone{zone(z, "", 1)}, "", StateDrift,
			[]Change{{Name: "www." + z, Type: "A", Kind: ChangeMissing, NetBox: &Side{300, []string{"192.0.2.1"}}}}},
		{"an extra RRset", []dns.Zone{zone(z, "v", 1)}, []dns.Zone{zone(z, "", 1, www(300, "192.0.2.1"))}, "", StateDrift,
			[]Change{{Name: "www." + z, Type: "A", Kind: ChangeExtra, PowerDNS: &Side{300, []string{"192.0.2.1"}}}}},
		{"a changed value", []dns.Zone{zone(z, "v", 1, www(300, "192.0.2.1", "192.0.2.2"))},
			[]dns.Zone{zone(z, "", 1, www(300, "192.0.2.1", "192.0.2.3"))}, "", StateDrift,
			[]Change{{Name: "www." + z, Type: "A", Kind: ChangeChanged,
				NetBox: &Side{300, []string{"192.0.2.1", "192.0.2.2"}}, PowerDNS: &Side{300, []string{"192.0.2.1", "192.0.2.3"}}}}},
		{"a changed TTL", []dns.Zone{zone(z, "v", 1, www(300, "192.0.2.1"))},
			[]dns.Zone{zone(z, "", 1, www(600, "192.0.2.1"))}, "", StateDrift,
			[]Change{{Name: "www." + z, Type: "A", Kind: ChangeChanged,
				NetBox: &Side{300, []string{"192.0.2.1"}}, PowerDNS: &Side{600, []string{"192.0.2.1"}}}}},
		{"an SOA that differs past its serial", []dns.Zone{zone(z, "v", 1)},
			[]dns.Zone{{Name: z, Active: true, SOASerial: 7, RRsets: []dns.RRset{rrset(z, "SOA", 3600,
				rec("ns1.example.com. admin.example.com. 7 10800 3600 604800 3600"))}}}, "", StateDrift,
			[]Change{{Name: z, Type: "SOA", Kind: ChangeChanged,
				NetBox:   &Side{3600, []string{fmt.Sprintf(soa, 1)}},
				PowerDNS: &Side{3600, []string{"ns1.example.com. admin.example.com. 7 10800 3600 604800 3600"}}}}},
		{"records neither side serves", []dns.Zone{zone(z, "v", 1, rrset("old."+z, "A", 60, off("192.0.2.9")))},
			[]dns.Zone{zone(z, "", 1, rrset("old."+z, "A", 120, off("192.0.2.8")))}, "", StateInSync, []Change{}},
		{"a record only NetBox doesn't serve", []dns.Zone{zone(z, "v", 1, rrset("www."+z, "A", 300, rec("192.0.2.1"), off("192.0.2.2")))},
			[]dns.Zone{zone(z, "", 1, www(300, "192.0.2.1", "192.0.2.2"))}, "", StateDrift,
			[]Change{{Name: "www." + z, Type: "A", Kind: ChangeChanged,
				NetBox: &Side{300, []string{"192.0.2.1"}}, PowerDNS: &Side{300, []string{"192.0.2.1", "192.0.2.2"}}}}},
		{"a zone missing on the primary", []dns.Zone{zone(z, "v", 1)}, nil, "", StateMissing, []Change{}},
		{"a parked zone the primary serves", []dns.Zone{parked}, []dns.Zone{zone(z, "", 1)}, "", StateInactive, []Change{}},
		{"an ignored zone", []dns.Zone{zone(z, "v", 1, www(300, "192.0.2.1"))}, []dns.Zone{zone(z, "", 1)},
			config.PolicyIgnore, StateIgnored, []Change{}},
		{"an enforced zone is reported", []dns.Zone{zone(z, "v", 1, www(300, "192.0.2.1"))}, []dns.Zone{zone(z, "", 1)},
			config.PolicyEnforce, StateDrift,
			[]Change{{Name: "www." + z, Type: "A", Kind: ChangeMissing, NetBox: &Side{300, []string{"192.0.2.1"}}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := config.Group{Name: "site-a", Views: []string{"v"}, DriftPolicy: config.PolicyReport}
			if tt.policy != "" {
				g.ZonePolicies = map[string]string{z: tt.policy}
			}
			r := Compare(g, tt.nb, tt.pd, nil)
			if len(r.Zones) != 1 {
				t.Fatalf("zones = %+v", r.Zones)
			}
			zr := r.Zones[0]
			if zr.State != tt.wantState || !reflect.DeepEqual(zr.Changes, tt.wantChanges) {
				t.Errorf("state %s, changes %s; want %s, %s", zr.State, show(zr.Changes), tt.wantState, show(tt.wantChanges))
			}
			if want := g.Policy(z); zr.Policy != want {
				t.Errorf("policy = %s, want %s", zr.Policy, want)
			}
		})
	}
}

func show(cs []Change) string {
	var parts []string
	for _, c := range cs {
		parts = append(parts, fmt.Sprintf("%s %s %s nb=%v pd=%v", c.Name, c.Type, c.Kind, c.NetBox, c.PowerDNS))
	}
	return "[" + strings.Join(parts, "; ") + "]"
}

func TestCompareGroup(t *testing.T) {
	parked := zone("parked.example.", "v", 1)
	parked.Active = false
	nb := []dns.Zone{
		zone("a.example.", "v", 1),
		zone("b.example.", "v", 1),
		zone("shared.example.", "w", 1),
		zone("shared.example.", "v", 1),
		parked,
	}
	pd := []dns.Zone{zone("a.example.", "", 1), zone("shared.example.", "", 1), zone("zz.example.", "", 1), zone("other.example.", "", 1)}
	g := config.Group{Name: "site-a", Views: []string{"v", "w"}, DriftPolicy: config.PolicyReport,
		ZonePolicies: map[string]string{"gone.example.": config.PolicyIgnore}}
	probs := []dns.Problem{{Zone: "a.example.", Detail: "kept"}, {Zone: "elsewhere.example.", Detail: "dropped"}}
	r := Compare(g, nb, pd, probs)

	var states []string
	for _, z := range r.Zones {
		states = append(states, z.Zone+"="+z.State+"@"+z.View)
	}
	// The parked zone isn't on the primary, so it's absent as expected and
	// not listed. Of the two shared zones, the one in view v is compared.
	want := []string{"a.example.=in_sync@v", "b.example.=missing@v", "shared.example.=in_sync@v"}
	if !slices.Equal(states, want) {
		t.Errorf("zones %v, want %v", states, want)
	}
	if !slices.Equal(r.Unmanaged, []string{"other.example.", "zz.example."}) {
		t.Errorf("unmanaged %v", r.Unmanaged)
	}
	if len(r.Problems) != 1 || r.Problems[0].Detail != "kept" {
		t.Errorf("problems %v", r.Problems)
	}
	if len(r.Warnings) != 2 || !strings.Contains(r.Warnings[0], "shared.example. is in the views v and w") ||
		!strings.Contains(r.Warnings[1], "zone_policies names gone.example.") {
		t.Errorf("warnings %q", r.Warnings)
	}
	if r.Counts != (Counts{InSync: 2, Missing: 1, Unmanaged: 2}) || !r.Counts.Drifted() || r.Status != StatusOK {
		t.Errorf("counts %+v, status %s", r.Counts, r.Status)
	}
}

// memory is a NetBox and a Primary that serve zones from memory, and count
// the zones they're asked to read.
type memory struct {
	zones   []dns.Zone
	err     error
	readErr error
	read    []string
}

func (m *memory) list(zone string) []dns.Zone {
	var out []dns.Zone
	for _, z := range m.zones {
		if zone == "" || z.Name == zone {
			bare := z
			bare.RRsets = nil
			out = append(out, bare)
		}
	}
	return out
}

func (m *memory) Read(_ context.Context, zones []dns.Zone) ([]dns.Zone, []dns.Problem, error) {
	if m.readErr != nil {
		return nil, nil, m.readErr
	}
	var out []dns.Zone
	for _, want := range zones {
		for _, z := range m.zones {
			if z.Name == want.Name && z.View == want.View {
				out = append(out, z)
				m.read = append(m.read, z.View+"/"+z.Name)
			}
		}
	}
	return out, []dns.Problem{}, nil
}

type memNetBox struct{ *memory }

func (m memNetBox) Zones(_ context.Context, views []string, zone string) ([]dns.Zone, error) {
	if m.err != nil {
		return nil, m.err
	}
	var out []dns.Zone
	for _, z := range m.list(zone) {
		if slices.Contains(views, z.View) {
			out = append(out, z)
		}
	}
	return out, nil
}

type memPrimary struct{ *memory }

func (m memPrimary) Zones(_ context.Context, zone string) ([]dns.Zone, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.list(zone), nil
}

func TestRun(t *testing.T) {
	nb := &memory{zones: []dns.Zone{
		zone("a.example.", "v", 1, rrset("www.a.example.", "A", 300, rec("192.0.2.1"))),
		zone("b.example.", "v", 1),
		zone("c.example.", "w", 1),
	}}
	pdA := &memory{zones: []dns.Zone{zone("a.example.", "", 1), zone("b.example.", "", 1), zone("x.example.", "", 1)}}
	pdB := &memory{zones: []dns.Zone{zone("c.example.", "", 1)}}
	groups := []Group{
		{Config: config.Group{Name: "site-a", Views: []string{"v"}, DriftPolicy: config.PolicyReport,
			ZonePolicies: map[string]string{"b.example.": config.PolicyIgnore}}, Primary: memPrimary{pdA}},
		{Config: config.Group{Name: "site-b", Views: []string{"w"}, DriftPolicy: config.PolicyReport}, Primary: memPrimary{pdB}},
		{Config: config.Group{Name: "site-c", Views: []string{"v"}, DriftPolicy: config.PolicyReport,
			ZonePolicies: map[string]string{"b.example.": config.PolicyIgnore}}, Err: errors.New("no client")},
	}
	r, err := Run(t.Context(), memNetBox{nb}, groups, "")
	if err != nil {
		t.Fatal(err)
	}
	if r.Complete || !r.Drift || len(r.Groups) != 3 {
		t.Fatalf("report complete %v, drift %v, %d groups", r.Complete, r.Drift, len(r.Groups))
	}
	a, b, c := r.Groups[0], r.Groups[1], r.Groups[2]
	if a.Counts != (Counts{Drift: 1, Ignored: 1, Unmanaged: 1}) || b.Counts != (Counts{InSync: 1}) {
		t.Errorf("counts %+v, %+v", a.Counts, b.Counts)
	}
	if c.Status != StatusFailed || c.Error != "no client" {
		t.Errorf("site-c: %+v", c)
	}
	// Only the zones compared are read: not the one every group ignores, nor
	// the primary's unmanaged one. NetBox's are read once, though two groups
	// serve view v.
	if slices.Sort(nb.read); !slices.Equal(nb.read, []string{"v/a.example.", "w/c.example."}) {
		t.Errorf("NetBox read %v", nb.read)
	}
	if !slices.Equal(pdA.read, []string{"/a.example."}) {
		t.Errorf("site-a's primary read %v", pdA.read)
	}

	t.Run("one zone", func(t *testing.T) {
		r, err := Run(t.Context(), memNetBox{nb}, groups[:2], "c.example.")
		if err != nil || !r.Complete || r.Drift || len(r.Groups[0].Zones) != 0 || len(r.Groups[1].Zones) != 1 {
			t.Errorf("report %+v, %v", r, err)
		}
	})
	t.Run("a zone that's nowhere", func(t *testing.T) {
		var nf *ZoneNotFoundError
		if _, err := Run(t.Context(), memNetBox{nb}, groups[:2], "nothere.example."); !errors.As(err, &nf) || nf.Zone != "nothere.example." {
			t.Errorf("Run: %v, want a ZoneNotFoundError", err)
		}
	})
	t.Run("a zone only on a primary", func(t *testing.T) {
		r, err := Run(t.Context(), memNetBox{nb}, groups[:2], "x.example.")
		if err != nil || !slices.Equal(r.Groups[0].Unmanaged, []string{"x.example."}) {
			t.Errorf("report %+v, %v", r, err)
		}
	})
	t.Run("a zone that's nowhere, with a group that can't be read", func(t *testing.T) {
		// The group that failed might have it, so the report is incomplete,
		// not wrong.
		r, err := Run(t.Context(), memNetBox{nb}, groups, "nothere.example.")
		if err != nil || r.Complete {
			t.Errorf("report %+v, %v", r, err)
		}
	})
	t.Run("NetBox can't be read", func(t *testing.T) {
		if _, err := Run(t.Context(), memNetBox{&memory{err: errors.New("down")}}, groups, ""); err == nil || err.Error() != "down" {
			t.Errorf("Run: %v, want NetBox's error", err)
		}
	})
	t.Run("a primary that fails while reading", func(t *testing.T) {
		broken := []Group{{Config: groups[0].Config, Primary: memPrimary{&memory{zones: pdA.zones, readErr: errors.New("reset")}}}}
		r, err := Run(t.Context(), memNetBox{nb}, broken, "")
		if err != nil || r.Complete || r.Groups[0].Status != StatusFailed || r.Groups[0].Error != "reset" {
			t.Errorf("report %+v, %v", r, err)
		}
	})
}
