package drift

import (
	"cmp"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
)

// sortedReport returns g with its problems and warnings sorted, which a full
// comparison lists in the order it read the zones, and without Compared.
func sortedReport(g GroupReport) GroupReport {
	g.Compared = nil
	g.Problems = slices.Clone(g.Problems)
	slices.SortFunc(g.Problems, func(a, b dns.Problem) int {
		return cmp.Or(strings.Compare(a.Zone, b.Zone), strings.Compare(a.Detail, b.Detail))
	})
	g.Warnings = slices.Clone(g.Warnings)
	slices.SortFunc(g.Warnings, func(a, b Warning) int { return strings.Compare(a.Text, b.Text) })
	return g
}

// TestZoneRefresh checks that a zone refresh, merged into the last full
// report, gives the report that a full refresh would.
func TestZoneRefresh(t *testing.T) {
	nb := &memory{zones: []dns.Zone{
		zone("a.example.", "v", 1, rrset("www.a.example.", "A", 300, rec("192.0.2.1"))),
		zone("b.example.", "v", 1),
		zone("c.example.", "w", 1, rrset("www.c.example.", "A", 300, rec("192.0.2.3"))),
		zone("d.example.", "v", 1, rrset("www.d.example.", "A", 300, rec("192.0.2.4"))),
		zone("shared.example.", "v", 1),
		zone("shared.example.", "w", 1),
	}, probs: []dns.Problem{{Zone: "a.example.", View: "v", Detail: "a problem in a"}, {Zone: "c.example.", View: "w", Detail: "a problem in c"}}}
	pdA := &memory{zones: []dns.Zone{
		zone("a.example.", "", 1, rrset("www.a.example.", "A", 300, rec("192.0.2.1"))),
		zone("b.example.", "", 1),
		zone("c.example.", "", 1, rrset("www.c.example.", "A", 300, rec("192.0.2.99"))),
		zone("d.example.", "", 1, rrset("www.d.example.", "A", 300, rec("192.0.2.4"))),
		zone("shared.example.", "", 1),
		zone("x.example.", "", 1),
	}}
	pdB := &memory{zones: []dns.Zone{zone("c.example.", "", 1, rrset("www.c.example.", "A", 300, rec("192.0.2.3")))}}
	groups := []Group{
		{Config: config.Group{Name: "site-a", Views: []string{"v", "w"}, DriftPolicy: config.PolicyReport,
			ZonePolicies: map[string]string{"b.example.": config.PolicyIgnore, "n.example.": config.PolicyIgnore}}, Primary: memPrimary{pdA}},
		{Config: config.Group{Name: "site-b", Views: []string{"w"}, DriftPolicy: config.PolicyReport}, Primary: memPrimary{pdB}},
	}
	before, err := Run(t.Context(), memNetBox{nb}, groups, Options{ReadNetBox: true})
	if err != nil || !before.Complete {
		t.Fatalf("the first full refresh: %+v, %v", before, err)
	}
	kept := sortedReport(before.Groups[0])

	// NetBox changes: a's record, d is deleted, n is created, which the
	// primary doesn't have, and shared leaves view w.
	nb.zones = []dns.Zone{
		zone("a.example.", "v", 1, rrset("www.a.example.", "A", 300, rec("192.0.2.2"))),
		zone("b.example.", "v", 1),
		zone("c.example.", "w", 1, rrset("www.c.example.", "A", 300, rec("192.0.2.3"))),
		zone("n.example.", "v", 1),
		zone("shared.example.", "v", 1),
	}
	nb.read, nb.lists, pdA.lists, pdB.lists = nil, nil, nil, nil
	refs := []ZoneRef{{"v", "a.example."}, {"v", "d.example."}, {"v", "n.example."}, {"w", "shared.example."}}
	r, err := Run(t.Context(), memNetBox{nb}, groups, Options{Zones: refs, ReadNetBox: true})
	if err != nil || !r.Complete || len(r.Groups) != 2 {
		t.Fatalf("the zone refresh: %+v, %v", r, err)
	}
	// NetBox is listed once, for the names. site-a's primary is listed in
	// full, once, for its four names; site-b's looks up its one.
	if !slices.Equal(nb.lists, []string{"a.example.,d.example.,n.example.,shared.example."}) ||
		!slices.Equal(pdA.lists, []string{""}) || !slices.Equal(pdB.lists, []string{"shared.example."}) {
		t.Errorf("lists: NetBox %q, site-a %q, site-b %q", nb.lists, pdA.lists, pdB.lists)
	}
	if slices.Sort(nb.read); !slices.Equal(nb.read, []string{"v/a.example.", "v/n.example.", "v/shared.example."}) {
		t.Errorf("NetBox read %v", nb.read)
	}
	if want := (&Scope{Views: []string{"v", "w"}, Names: []string{"a.example.", "d.example.", "n.example.", "shared.example."}}); !reflect.DeepEqual(r.Scope, want) {
		t.Errorf("scope %+v, want %+v", r.Scope, want)
	}
	if !slices.Equal(r.Groups[0].Compared, []string{"a.example.", "d.example.", "n.example.", "shared.example."}) ||
		!slices.Equal(r.Groups[1].Compared, []string{"shared.example."}) {
		t.Errorf("compared %q, %q", r.Groups[0].Compared, r.Groups[1].Compared)
	}

	after, err := Run(t.Context(), memNetBox{nb}, groups, Options{ReadNetBox: true})
	if err != nil {
		t.Fatal(err)
	}
	for i := range groups {
		got, want := sortedReport(Merge(before.Groups[i], r.Groups[i])), sortedReport(after.Groups[i])
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: merged\n%+v\nwant the full refresh's\n%+v", groups[i].Config.Name, got, want)
		}
	}
	// The last report is as it was.
	if !reflect.DeepEqual(sortedReport(before.Groups[0]), kept) {
		t.Error("Merge changed the last report")
	}
}

func TestZoneRefreshOfZonesNoGroupServes(t *testing.T) {
	nb := &memory{zones: []dns.Zone{zone("a.example.", "v", 1)}}
	pd := &memory{zones: nb.zones}
	groups := []Group{{Config: config.Group{Name: "site-a", Views: []string{"v"}, DriftPolicy: config.PolicyReport}, Primary: memPrimary{pd}}}
	r, err := Run(t.Context(), memNetBox{nb}, groups, Options{Zones: []ZoneRef{{"elsewhere", "a.example."}}, ReadNetBox: true})
	if err != nil || !r.Complete || len(r.Groups) != 0 || r.Scope == nil || len(nb.lists) != 0 || len(pd.lists) != 0 {
		t.Errorf("report %+v, %v; listed NetBox %q, the primary %q", r, err, nb.lists, pd.lists)
	}
}

func TestZoneRefreshOfOneZone(t *testing.T) {
	// One zone is looked up on each primary, not listed with every other.
	nb := &memory{zones: []dns.Zone{zone("a.example.", "v", 1)}}
	pd := &memory{zones: []dns.Zone{zone("a.example.", "", 1), zone("b.example.", "", 1)}}
	groups := []Group{{Config: config.Group{Name: "site-a", Views: []string{"v"}, DriftPolicy: config.PolicyReport}, Primary: memPrimary{pd}}}
	r, err := Run(t.Context(), memNetBox{nb}, groups, Options{Zones: []ZoneRef{{"v", "gone.example."}}})
	// A zone that neither side has was deleted, which isn't an error.
	if err != nil || len(r.Groups) != 1 || len(r.Groups[0].Zones) != 0 || len(r.Groups[0].Unmanaged) != 0 ||
		!slices.Equal(pd.lists, []string{"gone.example."}) {
		t.Errorf("report %+v, %v; the primary listed %q", r, err, pd.lists)
	}
}

func TestMergeCopies(t *testing.T) {
	old := GroupReport{Group: "g", Status: StatusOK,
		Zones:     make([]ZoneReport, 1, 10),
		Unmanaged: []string{}, Problems: []dns.Problem{}, Warnings: []Warning{}}
	old.Zones[0] = ZoneReport{Zone: "b.example.", State: StateInSync}
	r := GroupReport{Group: "g", Status: StatusOK, Compared: []string{"a.example."},
		Zones:     []ZoneReport{{Zone: "a.example.", State: StateMissing}},
		Unmanaged: []string{}, Problems: []dns.Problem{}, Warnings: []Warning{}}
	got := Merge(old, r)
	// Spare capacity in old's slice is never written to.
	if spare := old.Zones[:2]; spare[1].Zone != "" {
		t.Errorf("Merge wrote into the last report's slice: %+v", spare)
	}
	want := []ZoneReport{{Zone: "a.example.", State: StateMissing}, {Zone: "b.example.", State: StateInSync}}
	if !reflect.DeepEqual(got.Zones, want) || got.Counts != (Counts{InSync: 1, Missing: 1}) || got.Compared != nil {
		t.Errorf("merged %+v", got)
	}
}
