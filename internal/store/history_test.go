package store

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
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
	s := migrated(t)
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
		rows, events, err := s.RecordRefresh(ctx, full(t0.Add(time.Duration(i)*time.Hour), group(step.zones...)))
		if err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
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
	// The events are a valid trail, each linked to its refresh's.
	sum, err := s.VerifyAudit(ctx)
	if err != nil || sum.Count != 3 {
		t.Errorf("VerifyAudit() = %d events, %v, want 3", sum.Count, err)
	}
	var n int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM drift_zone_history").Scan(&n); err != nil || n != 6 {
		t.Errorf("%d zone rows, %v, want 6", n, err)
	}
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

func TestRestoreDrift(t *testing.T) {
	s := migrated(t)
	ctx := t.Context()
	empty, err := s.RestoreDrift(ctx)
	if err != nil || empty.Last != nil || len(empty.Groups) != 0 || !empty.LastComplete.IsZero() {
		t.Fatalf("RestoreDrift() of a new database = %+v, %v, want nothing", empty, err)
	}

	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	ga := group(zone("a.example.", drift.StateDrift, change("192.0.2.9")))
	ga.Counts = drift.Counts{Drift: 1}
	r := full(t0, ga)
	r.Groups = append(r.Groups, GroupResult{Group: "lab-b", Status: drift.StatusFailed, Error: "connection refused"})
	if _, _, err := s.RecordRefresh(ctx, r); err != nil {
		t.Fatal(err)
	}
	// A failed refresh after it.
	failed := RefreshRecord{Kind: RefreshFull, Cause: CauseWebhook, Started: t0.Add(time.Hour), Finished: t0.Add(time.Hour + time.Second),
		Outcome: "failed", Error: "NetBox is down"}
	if _, _, err := s.RecordRefresh(ctx, failed); err != nil {
		t.Fatal(err)
	}

	got, err := s.RestoreDrift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantLast := RefreshSummary{Kind: RefreshFull, Cause: CauseWebhook, Started: failed.Started, Finished: failed.Finished,
		Outcome: "failed", Error: "NetBox is down"}
	if got.Last == nil || *got.Last != wantLast || !got.LastComplete.Equal(t0) {
		t.Errorf("RestoreDrift() last = %+v, last complete %v, want %+v and %v", got.Last, got.LastComplete, wantLast, t0)
	}
	a := got.Groups["lab-a"]
	if a.Report == nil || !reflect.DeepEqual(*a.Report, ga) || !a.LastSuccess.Equal(t0) || a.Status != drift.StatusOK {
		t.Errorf("RestoreDrift() lab-a = %+v, want its report, with its warning's zone, from %v, ok", a, t0)
	}
	if b := got.Groups["lab-b"]; b.Report != nil || b.Status != drift.StatusFailed || b.Error != "connection refused" {
		t.Errorf("RestoreDrift() lab-b = %+v, want no report, and failed", b)
	}
}

func TestPruneDriftHistory(t *testing.T) {
	s := migrated(t)
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
		if _, _, err := s.RecordRefresh(ctx, r); err != nil {
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
}
