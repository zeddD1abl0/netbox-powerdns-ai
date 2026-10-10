package api

import (
	"net/http"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/api/gen"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
)

// historyStore returns a database whose history holds site-a's refreshes:
// example.com. in sync, then drifted, then a refresh that couldn't read
// NetBox, then a zone refresh that found example.com. in sync again.
func historyStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "nbpdns.db"), store.Options{Create: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.Migrate(t.Context(), "test"); err != nil {
		t.Fatal(err)
	}
	t0 := at("2026-10-08T00:00:00Z")
	zone := func(state string, changes ...drift.Change) drift.GroupReport {
		return drift.GroupReport{Group: "site-a", Status: drift.StatusOK, Counts: drift.Counts{InSync: 1},
			Zones: []drift.ZoneReport{{Zone: "example.com.", View: "_default_", Policy: "report", State: state,
				NetBoxSerial: 2, PowerDNSSerial: 1, Changes: changes}}}
	}
	full := func(at time.Time, g drift.GroupReport) store.RefreshRecord {
		return store.RefreshRecord{Kind: store.RefreshFull, Cause: store.CauseSchedule, Started: at, Finished: at.Add(2 * time.Second),
			Outcome: "complete", TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
			Groups:  []store.GroupResult{{Group: g.Group, Status: drift.StatusOK, Counts: g.Counts}},
			Reports: []store.KeptReport{{Report: g, LastSuccess: at.Add(2 * time.Second)}}}
	}
	changed := drift.Change{Name: "www.example.com.", Type: "A", Kind: drift.ChangeChanged,
		NetBox: &drift.Side{TTL: 300, Values: []string{"192.0.2.10"}}, PowerDNS: &drift.Side{TTL: 300, Values: []string{"192.0.2.99"}}}
	zones := full(t0.Add(3*time.Hour), zone(drift.StateInSync))
	zones.Kind, zones.Cause, zones.Zones, zones.NetBoxRequestIDs = store.RefreshZones, store.CauseWebhook, []string{"_default_/example.com."}, []string{"r1"}
	for _, r := range []store.RefreshRecord{
		full(t0, zone(drift.StateInSync)),
		full(t0.Add(time.Hour), zone(drift.StateDrift, changed)),
		{Kind: store.RefreshFull, Cause: store.CauseSchedule, Started: t0.Add(2 * time.Hour), Finished: t0.Add(2 * time.Hour),
			Outcome: "failed", Error: "couldn't read NetBox"},
		zones,
	} {
		if _, _, err := st.RecordRefresh(t.Context(), r); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

func TestListZoneHistory(t *testing.T) {
	o := Options{Source: zoned(), History: historyStore(t)}
	// Named without its final dot.
	r := getWith(t, o, http.MethodGet, "/api/server-groups/site-a/zones/example.com/history?limit=1")
	if r.code != http.StatusOK {
		t.Fatalf("%d: %s", r.code, r.body)
	}
	page := decode[gen.ZoneHistoryPage](t, r)
	if len(page.Items) != 1 || page.Items[0].State != gen.HistoryStateInSync || page.Next == nil {
		t.Fatalf("the first page = %+v, want the newest row, in sync, and a next page", page)
	}
	next, err := url.Parse(*page.Next)
	if err != nil {
		t.Fatal(err)
	}
	r = getWith(t, o, http.MethodGet, next.RequestURI())
	page = decode[gen.ZoneHistoryPage](t, r)
	if len(page.Items) != 1 || page.Next == nil {
		t.Fatalf("the second page = %+v", page)
	}
	e := page.Items[0]
	if e.State != gen.HistoryStateDrift || e.ChangeCount != 1 || len(e.Changes) != 1 || e.Changes[0].Powerdns.Values[0] != "192.0.2.99" ||
		*e.NetboxSerial != 2 || *e.PowerdnsSerial != 1 || e.DriftPolicy != gen.DriftPolicyReport || e.View != "_default_" ||
		!e.Time.Equal(at("2026-10-08T01:00:02Z")) || e.RefreshId != 2 {
		t.Errorf("the drifted row = %+v", e)
	}

	// A zone with no history has an empty page.
	r = getWith(t, o, http.MethodGet, "/api/server-groups/site-a/zones/nothing.example/history")
	if page := decode[gen.ZoneHistoryPage](t, r); r.code != http.StatusOK || len(page.Items) != 0 || page.Next != nil {
		t.Errorf("a zone with no history: %d %s", r.code, r.body)
	}
}

func TestListRefreshes(t *testing.T) {
	o := Options{Source: zoned(), History: historyStore(t)}
	r := getWith(t, o, http.MethodGet, "/api/server-groups/site-a/refreshes?limit=3")
	if r.code != http.StatusOK {
		t.Fatalf("%d: %s", r.code, r.body)
	}
	page := decode[gen.RefreshPage](t, r)
	if len(page.Items) != 3 || page.Next == nil {
		t.Fatalf("the page = %+v, want 3 and a next page", page)
	}
	z, failed := page.Items[0], page.Items[1]
	if z.Kind != gen.RefreshEntryKindZones || z.Cause != gen.RefreshEntryCauseWebhook || z.Zones == nil || (*z.Zones)[0] != "_default_/example.com." ||
		z.NetboxRequestIds[0] != "r1" || z.Group == nil || z.Group.Status != gen.GroupResultStatusOk || z.Group.Counts.InSync != 1 {
		t.Errorf("the zone refresh = %+v", z)
	}
	if failed.Outcome != gen.RefreshEntryOutcomeFailed || failed.Error == nil || failed.Group != nil || failed.Zones != nil {
		t.Errorf("the failed refresh = %+v, want no result for site-a", failed)
	}
	// Another group, never in the history, sees the refreshes without a
	// result; and a history-less API has none.
	r = getWith(t, o, http.MethodGet, "/api/server-groups/site-c/refreshes")
	if page := decode[gen.RefreshPage](t, r); len(page.Items) != 4 || page.Items[0].Group != nil {
		t.Errorf("site-c's refreshes = %s", r.body)
	}
	r = get(t, zoned(), http.MethodGet, "/api/server-groups/site-a/refreshes")
	if page := decode[gen.RefreshPage](t, r); r.code != http.StatusOK || len(page.Items) != 0 {
		t.Errorf("without a history: %d %s", r.code, r.body)
	}
}

func TestHistoryProblems(t *testing.T) {
	o := Options{Source: zoned(), History: historyStore(t)}
	for _, tt := range []struct {
		target string
		code   int
	}{
		{"/api/server-groups/nope/zones/example.com/history", http.StatusNotFound},
		{"/api/server-groups/site-a/zones/b%C3%BCcher.example/history", http.StatusNotFound},
		{"/api/server-groups/nope/refreshes", http.StatusNotFound},
		{"/api/server-groups/site-a/zones/example.com/history?limit=0", http.StatusBadRequest},
		{"/api/server-groups/site-a/refreshes?cursor=bogus", http.StatusBadRequest},
		{"/api/server-groups/site-a/refreshes?cursor=" + cursor{After: "x"}.String(), http.StatusBadRequest},
		{"/api/server-groups/site-a/zones/example.com/history?cursor=" + cursor{After: "-3"}.String(), http.StatusBadRequest},
	} {
		t.Run(tt.target, func(t *testing.T) {
			problemOf(t, getWith(t, o, http.MethodGet, tt.target), tt.code)
		})
	}
}
