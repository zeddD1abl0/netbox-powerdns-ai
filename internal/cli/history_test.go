package cli

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store"
)

// historyDB returns the URL of a database whose history holds two refreshes
// of site-a: example.com. in sync, then drifted.
func historyDB(t *testing.T) string {
	t.Helper()
	db := filepath.Join(t.TempDir(), "nbpdns.db")
	st, err := store.Open(t.Context(), db, store.Options{Create: true})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.Migrate(t.Context(), "test"); err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	for i, z := range []drift.ZoneReport{
		{Zone: "example.com.", View: "_default_", Policy: "report", State: drift.StateInSync, NetBoxSerial: 1, PowerDNSSerial: 1},
		{Zone: "example.com.", View: "_default_", Policy: "report", State: drift.StateDrift, NetBoxSerial: 2, PowerDNSSerial: 1,
			Changes: []drift.Change{{Name: "www.example.com.", Type: "A", Kind: drift.ChangeMissing, NetBox: &drift.Side{TTL: 300, Values: []string{"192.0.2.1"}}}}},
	} {
		at := t0.Add(time.Duration(i) * time.Hour)
		g := drift.GroupReport{Group: "site-a", Status: drift.StatusOK, Zones: []drift.ZoneReport{z}}
		g.Counts.InSync, g.Counts.Drift = 1-i, i
		if _, _, err := st.RecordRefresh(t.Context(), store.RefreshRecord{
			Kind: store.RefreshFull, Cause: store.CauseSchedule, Started: at, Finished: at.Add(time.Second), Outcome: "complete",
			Groups:  []store.GroupResult{{Group: "site-a", Status: drift.StatusOK, Counts: g.Counts}},
			Reports: []store.KeptReport{{Report: g, LastSuccess: at.Add(time.Second)}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	return "sqlite://" + db
}

func TestDriftHistory(t *testing.T) {
	env := map[string]string{"NBPDNS_DATABASE_URL": historyDB(t)}

	code, stdout, stderr := run(t, env, "drift", "history", "--group", "site-a", "--zone", "Example.com")
	for _, re := range []string{
		`(?m)^TIME +STATE +CHANGES +POLICY +NETBOX SERIAL +POWERDNS SERIAL +REFRESH$`,
		`(?m)^2026-10-08T00:00:01Z +in_sync +0 +report +1 +1 +1\n2026-10-08T01:00:01Z +drift +1 +report +2 +1 +2$`,
	} {
		if !regexp.MustCompile(re).MatchString(stdout) {
			t.Errorf("drift history --zone: exit %d, no match for %s:\n%s%s", code, re, stdout, stderr)
		}
	}

	code, stdout, stderr = run(t, env, "drift", "history", "--group", "site-a", "--zone", "example.com", "-o", "json")
	var rows []zoneHistoryRow
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil || code != exitOK || len(rows) != 2 {
		t.Fatalf("drift history --zone -o json: exit %d, %v:\n%s%s", code, err, stdout, stderr)
	}
	if rows[1].State != drift.StateDrift || len(rows[1].Changes) != 1 || rows[1].Changes[0].Kind != drift.ChangeMissing || rows[0].Changes == nil {
		t.Errorf("rows = %+v", rows)
	}

	code, stdout, stderr = run(t, env, "drift", "history", "--group", "site-a", "--limit", "1")
	if code != exitOK || !regexp.MustCompile(`(?m)^ID +FINISHED +CAUSE +OUTCOME +GROUP +DRIFTED +ZONES\n2 +2026-10-08T01:00:01Z +schedule +complete +ok +1 +every zone$`).MatchString(stdout) {
		t.Errorf("drift history --limit 1: exit %d:\n%s%s", code, stdout, stderr)
	}
	code, stdout, _ = run(t, env, "drift", "history", "--group", "site-a", "-o", "json")
	var refreshes []refreshRow
	if err := json.Unmarshal([]byte(stdout), &refreshes); err != nil || code != exitOK || len(refreshes) != 2 ||
		refreshes[0].ID != 1 || refreshes[1].Group == nil || refreshes[1].Group.Counts.Drift != 1 {
		t.Errorf("drift history -o json: exit %d, %v:\n%s", code, err, stdout)
	}

	code, stdout, _ = run(t, env, "drift", "history", "--group", "site-a", "--zone", "nothing.example")
	if code != exitOK || stdout != "Server group site-a has no history for nothing.example..\n" {
		t.Errorf("a zone with no history: exit %d: %q", code, stdout)
	}
	for _, args := range [][]string{{"drift", "history"}, {"drift", "history", "--group", "site-a", "--limit", "0"}} {
		if code, _, stderr := run(t, env, args...); code != exitUsage {
			t.Errorf("%s: exit %d, want %d:\n%s", strings.Join(args, " "), code, exitUsage, stderr)
		}
	}
}
