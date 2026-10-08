package drift

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
	probs := []dns.Problem{
		{Zone: "a.example.", Detail: "kept"},
		{Zone: "a.example.", View: "v", Detail: "kept from NetBox"},
		{Zone: "elsewhere.example.", Detail: "dropped"},
		// Not the shared zone compared, the one in v.
		{Zone: "shared.example.", View: "w", Detail: "dropped from the other view"},
	}
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
	if len(r.Problems) != 2 || r.Problems[0].Detail != "kept" || r.Problems[1].Detail != "kept from NetBox" {
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
	// probs are the problems that Read finds in the zones it reads.
	probs []dns.Problem
	mu    sync.Mutex
	read  []string
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
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []dns.Zone
	probs := []dns.Problem{}
	for _, want := range zones {
		for _, z := range m.zones {
			if z.Name == want.Name && z.View == want.View {
				out = append(out, z)
				m.read = append(m.read, z.View+"/"+z.Name)
				for _, p := range m.probs {
					if p.Zone == z.Name && p.View == z.View {
						probs = append(probs, p)
					}
				}
			}
		}
	}
	return out, probs, nil
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
		zone("d.example.", "v", 1, rrset("www.d.example.", "A", 300, rec("192.0.2.1"))),
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
	r, err := Run(t.Context(), memNetBox{nb}, groups, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Complete || !r.Drift || len(r.Groups) != 3 {
		t.Fatalf("report complete %v, drift %v, %d groups", r.Complete, r.Drift, len(r.Groups))
	}
	a, b, c := r.Groups[0], r.Groups[1], r.Groups[2]
	if a.Counts != (Counts{Drift: 1, Missing: 1, Ignored: 1, Unmanaged: 1}) || b.Counts != (Counts{InSync: 1}) {
		t.Errorf("counts %+v, %+v", a.Counts, b.Counts)
	}
	if c.Status != StatusFailed || c.Error != "no client" {
		t.Errorf("site-c: %+v", c)
	}
	// Only the zones compared are read: not the one every group ignores, nor
	// the one missing on the primary, nor the primary's unmanaged one.
	// NetBox's are read once, though two groups serve view v.
	if slices.Sort(nb.read); !slices.Equal(nb.read, []string{"v/a.example.", "w/c.example."}) {
		t.Errorf("NetBox read %v", nb.read)
	}
	if !slices.Equal(pdA.read, []string{"/a.example."}) {
		t.Errorf("site-a's primary read %v", pdA.read)
	}

	t.Run("one zone", func(t *testing.T) {
		// site-a's policy for b.example. isn't warned about, as a zone it
		// doesn't serve: only c.example. was listed.
		r, err := Run(t.Context(), memNetBox{nb}, groups[:2], Options{Zone: "c.example."})
		if err != nil || !r.Complete || r.Drift || len(r.Groups[0].Zones) != 0 || len(r.Groups[1].Zones) != 1 ||
			len(r.Groups[0].Warnings) != 0 || len(r.Groups[1].Warnings) != 0 {
			t.Errorf("report %+v, %v", r, err)
		}
		// The zone's own policy still applies.
		r, err = Run(t.Context(), memNetBox{nb}, groups[:1], Options{Zone: "b.example."})
		if err != nil || len(r.Groups[0].Zones) != 1 || r.Groups[0].Zones[0].State != StateIgnored || len(r.Groups[0].Warnings) != 0 {
			t.Errorf("report %+v, %v", r, err)
		}
	})
	t.Run("every group failed", func(t *testing.T) {
		// Nothing is read from NetBox for groups that can't be compared.
		fresh := &memory{zones: nb.zones}
		down := []Group{{Config: groups[0].Config, Primary: memPrimary{&memory{err: errors.New("refused")}}}, groups[2]}
		r, err := Run(t.Context(), memNetBox{fresh}, down, Options{})
		if err != nil || r.Complete || len(fresh.read) != 0 || r.Groups[0].Error != "refused" {
			t.Errorf("report %+v, %v; NetBox read %v", r, err, fresh.read)
		}
	})
	t.Run("a zone that's nowhere", func(t *testing.T) {
		var nf *ZoneNotFoundError
		if _, err := Run(t.Context(), memNetBox{nb}, groups[:2], Options{Zone: "nothere.example."}); !errors.As(err, &nf) || nf.Zone != "nothere.example." {
			t.Errorf("Run: %v, want a ZoneNotFoundError", err)
		}
	})
	t.Run("a zone only on a primary", func(t *testing.T) {
		r, err := Run(t.Context(), memNetBox{nb}, groups[:2], Options{Zone: "x.example."})
		if err != nil || !slices.Equal(r.Groups[0].Unmanaged, []string{"x.example."}) {
			t.Errorf("report %+v, %v", r, err)
		}
	})
	t.Run("a zone that's nowhere, with a group that can't be read", func(t *testing.T) {
		// The group that failed might have it, so the report is incomplete,
		// not wrong.
		r, err := Run(t.Context(), memNetBox{nb}, groups, Options{Zone: "nothere.example."})
		if err != nil || r.Complete {
			t.Errorf("report %+v, %v", r, err)
		}
	})
	t.Run("NetBox can't be read", func(t *testing.T) {
		if _, err := Run(t.Context(), memNetBox{&memory{err: errors.New("down")}}, groups, Options{}); err == nil || err.Error() != "down" {
			t.Errorf("Run: %v, want NetBox's error", err)
		}
	})
	t.Run("a primary that fails while reading", func(t *testing.T) {
		broken := []Group{{Config: groups[0].Config, Primary: memPrimary{&memory{zones: pdA.zones, readErr: errors.New("reset")}}}}
		r, err := Run(t.Context(), memNetBox{nb}, broken, Options{})
		if err != nil || r.Complete || r.Groups[0].Status != StatusFailed || r.Groups[0].Error != "reset" {
			t.Errorf("report %+v, %v", r, err)
		}
	})
}

// hooked is a Primary that calls hooks before it lists or reads.
type hooked struct {
	memPrimary
	onZones, onRead func()
}

func (h hooked) Zones(ctx context.Context, zone string) ([]dns.Zone, error) {
	if h.onZones != nil {
		h.onZones()
	}
	return h.memPrimary.Zones(ctx, zone)
}

func (h hooked) Read(ctx context.Context, zones []dns.Zone) ([]dns.Zone, []dns.Problem, error) {
	if h.onRead != nil {
		h.onRead()
	}
	return h.memPrimary.Read(ctx, zones)
}

func TestRunConcurrently(t *testing.T) {
	nb := &memory{zones: []dns.Zone{zone("a.example.", "v", 1)}}
	group := func(name string, p Primary) Group {
		return Group{Config: config.Group{Name: name, Views: []string{"v"}, DriftPolicy: config.PolicyReport}, Primary: p}
	}
	names := func(r Report) []string {
		var out []string
		for _, g := range r.Groups {
			out = append(out, g.Group)
		}
		return out
	}

	t.Run("up to the limit at once", func(t *testing.T) {
		var inFlight, most atomic.Int32
		listing := func() {
			n := inFlight.Add(1)
			for {
				m := most.Load()
				if n <= m || most.CompareAndSwap(m, n) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			inFlight.Add(-1)
		}
		var groups []Group
		for _, n := range []string{"a", "b", "c", "d"} {
			groups = append(groups, group(n, hooked{memPrimary: memPrimary{&memory{zones: nb.zones}}, onZones: listing}))
		}
		for _, limit := range []int{1, 2} {
			most.Store(0)
			r, err := Run(t.Context(), memNetBox{nb}, groups, Options{Concurrency: limit})
			if err != nil || !slices.Equal(names(r), []string{"a", "b", "c", "d"}) {
				t.Fatalf("limit %d: report %v, %v", limit, names(r), err)
			}
			if got := int(most.Load()); got != limit {
				t.Errorf("limit %d: %d groups listed at once", limit, got)
			}
		}
	})

	t.Run("a slow group doesn't delay the others", func(t *testing.T) {
		// The first group's read waits for the second's to finish, which it
		// can only do if they run at once.
		secondRead := make(chan struct{})
		slow := hooked{memPrimary: memPrimary{&memory{zones: nb.zones}}, onRead: func() {
			select {
			case <-secondRead:
			case <-time.After(5 * time.Second):
				t.Error("the second group wasn't read while the first one waited")
			}
		}}
		fast := hooked{memPrimary: memPrimary{&memory{zones: nb.zones}}, onRead: func() { close(secondRead) }}
		r, err := Run(t.Context(), memNetBox{nb}, []Group{group("slow", slow), group("fast", fast)}, Options{Concurrency: 2})
		if err != nil || !slices.Equal(names(r), []string{"slow", "fast"}) || r.Groups[0].Counts.InSync != 1 || r.Groups[1].Counts.InSync != 1 {
			t.Errorf("report %+v, %v", r, err)
		}
	})
}

func TestRunReadNetBox(t *testing.T) {
	inactive := zone("e.example.", "v", 1)
	inactive.Active = false
	zones := []dns.Zone{
		zone("a.example.", "v", 1, rrset("www.a.example.", "A", 300, rec("192.0.2.1"))),
		zone("b.example.", "v", 1),
		zone("c.example.", "w", 1),
		zone("d.example.", "v", 1, rrset("www.d.example.", "A", 300, rec("192.0.2.1"))),
		inactive,
		zone("f.example.", "elsewhere", 1),
	}
	// A problem in a compared zone, and one in d.example., which site-a's
	// primary doesn't have, so it's read only for the API.
	probs := []dns.Problem{{Zone: "a.example.", View: "v", Detail: "compared"}, {Zone: "d.example.", View: "v", Detail: "only read"}}
	pdA := &memory{zones: []dns.Zone{zone("a.example.", "", 1), zone("b.example.", "", 1)}}
	groups := func() []Group {
		return []Group{
			{Config: config.Group{Name: "site-a", Views: []string{"v"}, DriftPolicy: config.PolicyReport,
				ZonePolicies: map[string]string{"b.example.": config.PolicyIgnore}}, Primary: memPrimary{pdA}},
			{Config: config.Group{Name: "site-b", Views: []string{"w"}, DriftPolicy: config.PolicyReport}, Err: errors.New("no client")},
		}
	}
	lean := &memory{zones: zones, probs: probs}
	without, err := Run(t.Context(), memNetBox{lean}, groups(), Options{})
	if err != nil || without.NetBox != nil {
		t.Fatalf("without ReadNetBox: %v, NetBox %v", err, without.NetBox)
	}
	full := &memory{zones: zones, probs: probs}
	with, err := Run(t.Context(), memNetBox{full}, groups(), Options{ReadNetBox: true})
	if err != nil {
		t.Fatal(err)
	}
	// The comparison is the same, the problem in d.example. left out.
	if !reflect.DeepEqual(with.Groups, without.Groups) {
		t.Errorf("the reports differ:\nwith    %+v\nwithout %+v", with.Groups, without.Groups)
	}
	if p := with.Groups[0].Problems; len(p) != 1 || p[0].Detail != "compared" {
		t.Errorf("site-a's problems %+v", p)
	}
	// Every active zone in the groups' views is read, even of a group that
	// failed and of zones not compared, but not the inactive one, nor one in
	// no group's view.
	var got []string
	for _, z := range with.NetBox {
		got = append(got, z.View+"/"+z.Name)
	}
	if want := []string{"v/a.example.", "v/b.example.", "v/d.example.", "w/c.example."}; !slices.Equal(got, want) {
		t.Errorf("NetBox zones %v, want %v", got, want)
	}
	if slices.Sort(full.read); !slices.Equal(full.read, []string{"v/a.example.", "v/b.example.", "v/d.example.", "w/c.example."}) {
		t.Errorf("NetBox read %v", full.read)
	}
	if with.NetBox[2].RRsets[1].Name != "www.d.example." {
		t.Errorf("d.example. without its RRsets: %+v", with.NetBox[2])
	}
}
