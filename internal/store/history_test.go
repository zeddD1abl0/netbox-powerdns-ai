package store

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/dns"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
)

// zone returns a zone's report.
func zone(name, state string, changes ...drift.Change) drift.ZoneReport {
	return drift.ZoneReport{Zone: name, View: "_default_", Policy: "report", State: state, NetBoxSerial: 7, PowerDNSSerial: 7,
		Changes: changes}
}

// group returns lab-a's report of zones.
func group(zones ...drift.ZoneReport) drift.GroupReport {
	return drift.GroupReport{Group: "lab-a", Status: drift.StatusOK, Zones: zones, Unmanaged: []string{},
		Warnings: []drift.Warning{{Zone: "a.example.", Text: "a warning about a.example."}}}
}

// full returns a complete full refresh at finished, which kept reports.
func full(finished time.Time, reports ...drift.GroupReport) RefreshRecord {
	r := RefreshRecord{Kind: RefreshFull, Cause: CauseSchedule, Started: finished.Add(-time.Second), Finished: finished,
		Outcome: "complete", TraceID: "t"}
	for _, g := range reports {
		r.Groups = append(r.Groups, GroupResult{Group: g.Group, Status: drift.StatusOK, Counts: g.Counts})
		r.Reports = append(r.Reports, KeptReport{Report: g, LastSuccess: finished})
	}
	return r
}

func change(value string) drift.Change {
	return drift.Change{Name: "www.b.example.", Type: "A", Kind: drift.ChangeChanged,
		NetBox: &drift.Side{TTL: 300, Values: []string{"192.0.2.1"}}, PowerDNS: &drift.Side{TTL: 300, Values: []string{value}}}
}

// TestZoneHistory drives a zone through drift and back, and out of its
// group's report and in again, and checks the rows and events each refresh
// writes.
func TestZoneHistory(t *testing.T) {
	onEach(t, func(t *testing.T, db backend) {
		s := db.migrated(t)
		mirror(t, s, "lab-a", "lab-b")
		ctx := t.Context()
		a := zone("a.example.", drift.StateInSync)
		t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		steps := []struct {
			name   string
			zones  []drift.ZoneReport
			rows   int
			events []string // each event's after state
		}{
			{"the first refresh writes every zone, and an event for the drifted one",
				[]drift.ZoneReport{a, zone("b.example.", drift.StateDrift, change("192.0.2.9"))}, 2, []string{"drift"}},
			{"no change writes nothing", []drift.ZoneReport{a, zone("b.example.", drift.StateDrift, change("192.0.2.9"))}, 0, nil},
			{"other changes, in the same state, write a row, and no event",
				[]drift.ZoneReport{a, zone("b.example.", drift.StateDrift, change("192.0.2.10"))}, 1, nil},
			{"back in sync", []drift.ZoneReport{a, zone("b.example.", drift.StateInSync)}, 1, []string{"in_sync"}},
			{"gone from the report", []drift.ZoneReport{a}, 1, []string{StateRemoved}},
			{"still gone", []drift.ZoneReport{a}, 0, nil},
			{"back, in sync, as a new zone", []drift.ZoneReport{a, zone("b.example.", drift.StateInSync)}, 1, nil},
		}
		for i, step := range steps {
			w, err := s.RecordRefresh(ctx, full(t0.Add(time.Duration(i)*time.Hour), group(step.zones...)))
			if err != nil {
				t.Fatalf("%s: %v", step.name, err)
			}
			rows, events := w.ZoneRows, w.Events
			var states []string
			for _, e := range events {
				if e.Action != audit.ZoneDriftChanged || e.Target != (audit.Target{Type: "zone", ID: "lab-a/b.example."}) {
					t.Errorf("%s: event %+v, want zone.drift_changed for lab-a/b.example.", step.name, e)
				}
				var after struct{ State string }
				if err := json.Unmarshal(e.After, &after); err != nil {
					t.Fatal(err)
				}
				states = append(states, after.State)
			}
			if rows != step.rows || !reflect.DeepEqual(states, step.events) {
				t.Errorf("%s: %d rows, events to %v, want %d rows, events to %v", step.name, rows, states, step.rows, step.events)
			}
		}
		// The events are a valid trail, each linked to its refresh's, after the
		// mirror's two.
		sum, err := s.VerifyAudit(ctx)
		if err != nil || sum.Count != 5 {
			t.Errorf("VerifyAudit() = %d events, %v, want 5", sum.Count, err)
		}
		var n int
		if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM drift_zone_history").Scan(&n); err != nil || n != 6 {
			t.Errorf("%d zone rows, %v, want 6", n, err)
		}
	})
}

func TestDigestIgnoresOrder(t *testing.T) {
	x, y := change("192.0.2.9"), change("192.0.2.9")
	y.Name = "mail.b.example."
	_, d1, err := digestOf([]drift.Change{x, y})
	if err != nil {
		t.Fatal(err)
	}
	_, d2, _ := digestOf([]drift.Change{y, x})
	_, d3, _ := digestOf([]drift.Change{x})
	if d1 != d2 || d1 == d3 || len(d1) != 96 {
		t.Errorf("digests %s, %s, %s: want the first two equal, and the third different", d1, d2, d3)
	}
}

// TestLoadDrift records refreshes, and loads them back: the last full
// refresh, whatever came after it, and each group's report and result.
func TestLoadDrift(t *testing.T) {
	onEach(t, func(t *testing.T, db backend) {
		s := db.migrated(t)
		mirror(t, s, "lab-a", "lab-b")
		ctx := t.Context()
		empty, err := s.LoadDrift(ctx, 0)
		if err != nil || empty.Version != 0 || empty.Last != nil || len(empty.Groups) != 0 || !empty.LastComplete.IsZero() ||
			empty.NetBox.Status != "" || !empty.NetBox.ZonesAsOf.IsZero() || len(empty.NetBox.Zones) != 0 {
			t.Fatalf("LoadDrift() of a new database = %+v, %v, want nothing", empty, err)
		}

		t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		ga := group(zone("a.example.", drift.StateDrift, change("192.0.2.9")))
		ga.Counts = drift.Counts{Drift: 1}
		r := full(t0, ga)
		r.Groups = append(r.Groups, GroupResult{Group: "lab-b", Status: drift.StatusFailed, Error: "connection refused"})
		first, err := s.RecordRefresh(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		// A failed refresh after it, then a zone refresh, which isn't the
		// last full refresh.
		failed := RefreshRecord{Kind: RefreshFull, Cause: CauseWebhook, Started: t0.Add(time.Hour), Finished: t0.Add(time.Hour + time.Second),
			Outcome: "failed", Error: "NetBox is down", NetBoxStatus: NetBoxDown, NetBoxError: "NetBox is down"}
		if _, err := s.RecordRefresh(ctx, failed); err != nil {
			t.Fatal(err)
		}
		zones := RefreshRecord{Kind: RefreshZones, Cause: CauseWebhook, Started: t0.Add(2 * time.Hour), Finished: t0.Add(2*time.Hour + time.Second),
			Outcome: "complete", Zones: []string{"_default_/a.example."}}
		last, err := s.RecordRefresh(ctx, zones)
		if err != nil {
			t.Fatal(err)
		}
		if first.ID != 1 || last.ID != 3 {
			t.Errorf("RecordRefresh() IDs = %d and %d, want 1 and 3", first.ID, last.ID)
		}

		got, err := s.LoadDrift(ctx, 0)
		if err != nil {
			t.Fatal(err)
		}
		wantLast := RefreshSummary{Kind: RefreshFull, Cause: CauseWebhook, Started: failed.Started, Finished: failed.Finished,
			Outcome: "failed", Error: "NetBox is down"}
		if got.Version != 3 || got.Last == nil || *got.Last != wantLast || !got.LastComplete.Equal(t0) {
			t.Errorf("LoadDrift() version %d, last = %+v, last complete %v, want 3, %+v and %v", got.Version, got.Last, got.LastComplete, wantLast, t0)
		}
		a := got.Groups["lab-a"]
		if a.Report == nil || !reflect.DeepEqual(*a.Report, ga) || !a.LastSuccess.Equal(t0) || a.Status != drift.StatusOK {
			t.Errorf("LoadDrift() lab-a = %+v, want its report, with its warning's zone, from %v, ok", a, t0)
		}
		if b := got.Groups["lab-b"]; b.Report != nil || b.Status != drift.StatusFailed || b.Error != "connection refused" {
			t.Errorf("LoadDrift() lab-b = %+v, want no report, and failed", b)
		}
		// The zone refresh said nothing of NetBox, which stays down.
		if nb := got.NetBox; nb.Status != NetBoxDown || nb.Error != "NetBox is down" {
			t.Errorf("LoadDrift() NetBox = %+v, want down, as the failed refresh found it", nb)
		}
		if v, err := s.Versions(ctx); err != nil || v.Refreshes != 3 {
			t.Errorf("Versions() refreshes = %d, %v, want 3", v.Refreshes, err)
		}

		// Since the version, there's nothing new to read.
		same, err := s.LoadDrift(ctx, 3)
		if err != nil || same.Version != 3 || same.Last != nil || len(same.Groups) != 0 {
			t.Errorf("LoadDrift(3) = %+v, %v, want only the version", same, err)
		}
	})
}

// nbZone returns NetBox's active zone name in view _default_, with an A
// record of value.
func nbZone(name, value string) dns.Zone {
	return dns.Zone{Name: name, View: "_default_", Status: "active", Active: true, SOASerial: 7, Nameservers: []string{"ns1.example."},
		RRsets: []dns.RRset{{Name: "www." + name, Type: "A", TTL: 300, Records: []dns.Record{{Value: value, TTL: 300, Status: "active", Active: true}}}}}
}

// TestNetBoxZones records NetBox's zones with full and zone refreshes, and
// checks that each writes the rows of the zones that changed only, and that
// a replica that holds them as of a version loads only those written since.
func TestNetBoxZones(t *testing.T) {
	onEach(t, func(t *testing.T, db backend) {
		s := db.migrated(t)
		mirror(t, s, "lab-a")
		ctx := t.Context()
		t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		record := func(r RefreshRecord) Recorded {
			t.Helper()
			w, err := s.RecordRefresh(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			return w
		}
		withZones := func(r RefreshRecord, sc *drift.Scope, zones ...dns.Zone) RefreshRecord {
			r.NetBoxStatus, r.NetBoxZones = NetBoxUp, &NetBoxZones{Zones: zones, Scope: sc}
			return r
		}
		a, b, c := nbZone("a.example.", "192.0.2.1"), nbZone("b.example.", "192.0.2.2"), nbZone("c.example.", "192.0.2.3")
		other := nbZone("a.example.", "198.51.100.1")
		other.View = "internal"

		w := record(withZones(full(t0), nil, a, b, other))
		if w.NetBoxZones != 3 {
			t.Errorf("the first full refresh wrote %d of NetBox's zones, want 3", w.NetBoxZones)
		}
		got, err := s.LoadDrift(ctx, 0)
		if err != nil {
			t.Fatal(err)
		}
		nb := got.NetBox
		if nb.Status != NetBoxUp || !nb.ZonesAsOf.Equal(t0) || len(nb.Zones) != 3 || len(nb.Index["_default_"]) != 2 || len(nb.Index["internal"]) != 1 {
			t.Fatalf("LoadDrift() NetBox = %+v, want up, as of %v, with 3 zones", nb, t0)
		}
		if i := slices.IndexFunc(nb.Zones, func(z dns.Zone) bool { return z.View == "_default_" && z.Name == "a.example." }); i < 0 ||
			!reflect.DeepEqual(nb.Zones[i], a) {
			t.Errorf("LoadDrift() a.example. = %+v, want %+v", nb.Zones, a)
		}

		// One zone changes, one goes, and one is new: three rows. The
		// unchanged one keeps its row.
		changed := nbZone("b.example.", "192.0.2.20")
		w = record(withZones(full(t0.Add(time.Hour)), nil, a, changed, c))
		if w.NetBoxZones != 3 {
			t.Errorf("a full refresh that changed b.example., removed internal's zone and added c.example. wrote %d rows, want 3", w.NetBoxZones)
		}
		got, err = s.LoadDrift(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		names := func(zones []dns.Zone) []string {
			var out []string
			for _, z := range zones {
				out = append(out, z.View+"/"+z.Name)
			}
			slices.Sort(out)
			return out
		}
		if n := names(got.NetBox.Zones); !slices.Equal(n, []string{"_default_/b.example.", "_default_/c.example."}) {
			t.Errorf("LoadDrift(1) zones = %v, want b.example. and c.example. only", n)
		}
		if ix := got.NetBox.Index; ix["_default_"]["a.example."] != 1 || ix["_default_"]["b.example."] != 2 || len(ix["internal"]) != 0 {
			t.Errorf("LoadDrift(1) index = %v, want a.example. from refresh 1, b.example. from 2, and nothing in internal", ix)
		}
		if !got.NetBox.ZonesAsOf.Equal(t0.Add(time.Hour)) {
			t.Errorf("zones as of %v, want the second full refresh's end", got.NetBox.ZonesAsOf)
		}

		// The same zones again write nothing.
		if w := record(withZones(full(t0.Add(2*time.Hour)), nil, a, changed, c)); w.NetBoxZones != 0 {
			t.Errorf("a full refresh with no change wrote %d rows, want 0", w.NetBoxZones)
		}

		// A zone refresh of a.example. and b.example. in _default_ changes
		// a.example., finds b.example. gone, and leaves c.example. and the
		// zones outside its views alone; its zones keep their time.
		sc := &drift.Scope{Views: []string{"_default_"}, Names: []string{"a.example.", "b.example."}}
		zr := RefreshRecord{Kind: RefreshZones, Cause: CauseWebhook, Started: t0.Add(3 * time.Hour), Finished: t0.Add(3*time.Hour + time.Second),
			Outcome: "complete"}
		elsewhere := nbZone("d.example.", "192.0.2.4")
		elsewhere.View = "internal"
		w = record(withZones(zr, sc, nbZone("a.example.", "192.0.2.10"), elsewhere))
		if w.NetBoxZones != 2 {
			t.Errorf("the zone refresh wrote %d rows, want 2: a.example.'s, and b.example.'s delete", w.NetBoxZones)
		}
		got, err = s.LoadDrift(ctx, 0)
		if err != nil {
			t.Fatal(err)
		}
		if n := names(got.NetBox.Zones); !slices.Equal(n, []string{"_default_/a.example.", "_default_/c.example."}) {
			t.Errorf("after the zone refresh, zones = %v, want a.example. and c.example.", n)
		}
		if !got.NetBox.ZonesAsOf.Equal(t0.Add(2 * time.Hour)) {
			t.Errorf("after the zone refresh, zones as of %v, want the last full refresh's end", got.NetBox.ZonesAsOf)
		}

		// A refresh that says nothing of NetBox, as one that ran out of
		// time, leaves its state; one that couldn't read it says so.
		record(RefreshRecord{Kind: RefreshFull, Cause: CauseSchedule, Started: t0.Add(4 * time.Hour), Finished: t0.Add(4 * time.Hour), Outcome: "failed",
			Error: "the refresh took longer than drift.timeout"})
		if got, err := s.LoadDrift(ctx, 0); err != nil || got.NetBox.Status != NetBoxUp {
			t.Errorf("after a timeout, NetBox = %+v, %v, want up still", got.NetBox, err)
		}
		record(RefreshRecord{Kind: RefreshFull, Cause: CauseSchedule, Started: t0.Add(5 * time.Hour), Finished: t0.Add(5 * time.Hour), Outcome: "failed",
			Error: "connection refused", NetBoxStatus: NetBoxDown, NetBoxError: "connection refused"})
		if got, err := s.LoadDrift(ctx, 0); err != nil || got.NetBox.Status != NetBoxDown || got.NetBox.Error != "connection refused" ||
			len(got.NetBox.Zones) != 2 {
			t.Errorf("after NetBox failed, NetBox = %+v, %v, want down, with its zones kept", got.NetBox, err)
		}
	})
}

func TestPruneDriftHistory(t *testing.T) {
	onEach(t, func(t *testing.T, db backend) {
		s := db.migrated(t)
		mirror(t, s, "lab-a", "lab-b")
		ctx := t.Context()
		now := time.Now().UTC()
		old := now.AddDate(0, 0, -10)
		steps := []RefreshRecord{
			full(old, group(zone("a.example.", drift.StateInSync), zone("b.example.", drift.StateInSync),
				zone("c.example.", drift.StateInSync))),
			full(old.Add(time.Hour), group(zone("a.example.", drift.StateInSync),
				zone("b.example.", drift.StateDrift, change("192.0.2.9")))),
			full(now, group(zone("a.example.", drift.StateInSync), zone("b.example.", drift.StateDrift, change("192.0.2.9")))),
		}
		for _, r := range steps {
			if _, err := s.RecordRefresh(ctx, r); err != nil {
				t.Fatal(err)
			}
		}
		refreshes, rows, err := s.PruneDriftHistory(ctx, now.AddDate(0, 0, -7))
		if err != nil {
			t.Fatal(err)
		}
		// a's only row and b's last stay; b's first, c's first, and c's removed
		// row go.
		if refreshes != 2 || rows != 3 {
			t.Errorf("PruneDriftHistory() = %d refreshes, %d rows, want 2 and 3", refreshes, rows)
		}
		var left []string
		qr, err := s.db.QueryContext(ctx, "SELECT zone || ' ' || state FROM drift_zone_history ORDER BY id")
		if err != nil {
			t.Fatal(err)
		}
		defer qr.Close()
		for qr.Next() {
			var z string
			if err := qr.Scan(&z); err != nil {
				t.Fatal(err)
			}
			left = append(left, z)
		}
		if err := qr.Err(); err != nil {
			t.Fatal(err)
		}
		if want := []string{"a.example. in_sync", "b.example. drift"}; !reflect.DeepEqual(left, want) {
			t.Errorf("rows left = %v, want %v", left, want)
		}
		var groups int
		if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM drift_refresh_groups").Scan(&groups); err != nil || groups != 1 {
			t.Errorf("%d group results left, %v, want the last refresh's 1", groups, err)
		}
	})
}

func TestReadHistory(t *testing.T) {
	onEach(t, func(t *testing.T, db backend) {
		s := db.migrated(t)
		mirror(t, s, "lab-a", "lab-b")
		ctx := t.Context()
		t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
		records := []RefreshRecord{
			full(t0, group(zone("b.example.", drift.StateInSync))),
			full(t0.Add(time.Hour), group(zone("b.example.", drift.StateDrift, change("192.0.2.9")))),
			{Kind: RefreshFull, Cause: CauseSchedule, Started: t0.Add(2 * time.Hour), Finished: t0.Add(2 * time.Hour),
				Outcome: "failed", Error: "NetBox is down"},
			{Kind: RefreshZones, Cause: CauseWebhook, Started: t0.Add(3 * time.Hour), Finished: t0.Add(3 * time.Hour),
				Outcome: "complete", Zones: []string{"_default_/b.example."}, NetBoxRequestIDs: []string{"r1"},
				Groups:  []GroupResult{{Group: "lab-a", Status: drift.StatusOK, Counts: drift.Counts{InSync: 1}}},
				Reports: []KeptReport{{Report: group(zone("b.example.", drift.StateInSync)), LastSuccess: t0.Add(time.Hour)}}},
		}
		for _, r := range records {
			if _, err := s.RecordRefresh(ctx, r); err != nil {
				t.Fatal(err)
			}
		}

		rows, err := s.ZoneHistory(ctx, "lab-a", "b.example.", 0, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 || rows[0].State != drift.StateInSync || !rows[0].At.Equal(t0.Add(3*time.Hour)) ||
			rows[1].State != drift.StateDrift || len(rows[1].Changes) != 1 || rows[1].Changes[0].PowerDNS.Values[0] != "192.0.2.9" ||
			rows[1].NetBoxSerial != 7 || rows[1].Policy != "report" {
			t.Errorf("the zone's newest two rows = %+v", rows)
		}
		older, err := s.ZoneHistory(ctx, "lab-a", "b.example.", rows[1].ID, 10)
		if err != nil || len(older) != 1 || older[0].State != drift.StateInSync || !older[0].At.Equal(t0) {
			t.Errorf("the rows before %d = %+v, %v, want the first", rows[1].ID, older, err)
		}
		if none, err := s.ZoneHistory(ctx, "lab-a", "nothing.example.", 0, 10); err != nil || len(none) != 0 {
			t.Errorf("a zone with no history = %+v, %v", none, err)
		}

		refreshes, err := s.Refreshes(ctx, "lab-a", 0, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(refreshes) != 4 {
			t.Fatalf("Refreshes() = %d, want 4", len(refreshes))
		}
		z, failed := refreshes[0], refreshes[1]
		if z.Kind != RefreshZones || z.Cause != CauseWebhook || !reflect.DeepEqual(z.Zones, []string{"_default_/b.example."}) ||
			!reflect.DeepEqual(z.NetBoxRequestIDs, []string{"r1"}) || z.Group == nil || z.Group.Counts.InSync != 1 {
			t.Errorf("the zone refresh = %+v", z)
		}
		if failed.Outcome != "failed" || failed.Error != "NetBox is down" || failed.Group != nil || failed.Zones != nil {
			t.Errorf("the failed refresh = %+v, want no result for lab-a, and no zones", failed)
		}
		if other, err := s.Refreshes(ctx, "lab-b", refreshes[1].ID, 10); err != nil || len(other) != 2 || other[0].Group != nil {
			t.Errorf("lab-b's refreshes before %d = %+v, %v, want 2, without a result", refreshes[1].ID, other, err)
		}
	})
}
