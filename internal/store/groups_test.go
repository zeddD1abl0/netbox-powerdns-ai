package store

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/secrets"
)

// testGroup returns a group named name.
func testGroup(name string) config.Group {
	return config.Group{Name: name, Views: []string{"_default_"}, DriftPolicy: config.PolicyReport,
		Primary: config.Primary{URL: "http://pdns.example:8081", ServerID: "localhost"}}
}

// mirror stores the file groups named names, as serve does at its start.
func mirror(t *testing.T, s *Store, names ...string) {
	t.Helper()
	groups := make([]config.Group, len(names))
	for i, n := range names {
		groups[i] = testGroup(n)
	}
	if _, err := s.MirrorFileGroups(t.Context(), groups, audit.File("nbpdns.yaml")); err != nil {
		t.Fatal(err)
	}
}

// sealer returns a Seal of key with k.
func sealer(k secrets.MasterKey, key string) Seal {
	return func(place string) (string, error) { return secrets.Seal(k, place, key) }
}

// groupEvents returns the actions of the events about server groups, and
// each one's target, before and after, as text.
func groupEvents(t *testing.T, s *Store) []string {
	t.Helper()
	events, err := s.ListAudit(t.Context(), AuditFilter{TargetType: "server-group"})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = string(e.Action) + " " + e.Target.ID + " " + e.Actor.String() + " " + string(e.Before) + " " + string(e.After)
	}
	return out
}

func TestMirrorFileGroups(t *testing.T) {
	s := migrated(t)
	ctx := t.Context()
	file := audit.File("/etc/nbpdns/nbpdns.yaml")
	a, b := testGroup("site-a"), testGroup("site-b")
	a.Primary.APIKey = config.NewSecret("file-key")
	changedA := a
	changedA.Views = []string{"_default_", "internal"}
	steps := []struct {
		name    string
		groups  []config.Group
		events  int
		version int64
	}{
		{"two new groups", []config.Group{a, b}, 2, 1},
		{"no change", []config.Group{a, b}, 0, 1},
		{"a changed view", []config.Group{changedA, b}, 1, 2},
		{"a group gone from the file", []config.Group{changedA}, 1, 3},
	}
	for _, step := range steps {
		events, err := s.MirrorFileGroups(ctx, step.groups, file)
		if err != nil || len(events) != step.events {
			t.Fatalf("%s: MirrorFileGroups() = %d events, %v, want %d", step.name, len(events), err, step.events)
		}
		if v, _ := s.GroupsVersion(ctx); v != step.version {
			t.Errorf("%s: version %d, want %d", step.name, v, step.version)
		}
	}
	got, version, err := s.Groups(ctx)
	if err != nil || version != 3 || len(got) != 1 {
		t.Fatalf("Groups() = %+v, %d, %v, want site-a alone", got, version, err)
	}
	// The file's key stays in the file.
	want := changedA
	want.ManagedBy, want.Primary.APIKey = config.ManagedByFile, config.Secret{}
	if g := got[0]; !reflect.DeepEqual(g.Group, want) || g.APIKey != "" || g.CreatedBy != file.String() || g.UpdatedBy != file.String() {
		t.Errorf("Groups() = %+v, want %+v, by the file, with no key", g, want)
	}
	wantEvents := []string{
		`group.created site-a file:/etc/nbpdns/nbpdns.yaml  {"drift_policy":"report","managed_by":"file","primary.server_id":"localhost","primary.url":"http://pdns.example:8081","views":["_default_"]}`,
		`group.created site-b file:/etc/nbpdns/nbpdns.yaml  {"drift_policy":"report","managed_by":"file","primary.server_id":"localhost","primary.url":"http://pdns.example:8081","views":["_default_"]}`,
		`group.changed site-a file:/etc/nbpdns/nbpdns.yaml {"views":["_default_"]} {"views":["_default_","internal"]}`,
		`group.removed site-b file:/etc/nbpdns/nbpdns.yaml {"drift_policy":"report","managed_by":"file","primary.server_id":"localhost","primary.url":"http://pdns.example:8081","views":["_default_"]} `,
	}
	if events := groupEvents(t, s); !reflect.DeepEqual(events, wantEvents) {
		t.Errorf("events:\n%s\nwant:\n%s", strings.Join(events, "\n"), strings.Join(wantEvents, "\n"))
	}

	// A name that the CLI holds stops the mirror, and changes nothing.
	k := secrets.NewMasterKey()
	if _, err := s.AddGroup(ctx, testGroup("site-c"), sealer(k, "cli-key"), Change{Actor: audit.Actor{Kind: audit.ActorOSUser, ID: "jordan"}}); err != nil {
		t.Fatal(err)
	}
	_, err = s.MirrorFileGroups(ctx, []config.Group{testGroup("site-c")}, file)
	var ce *ConflictError
	if !errors.As(err, &ce) || ce.Name != "site-c" || ce.AddedBy != "os-user:jordan" ||
		!strings.Contains(err.Error(), "server group site-c is in the config file, and os-user:jordan added a group of that name") {
		t.Errorf("a conflict: %v", err)
	}
	if got, _, _ := s.Groups(ctx); len(got) != 2 || got[0].Group.Name != "site-a" {
		t.Errorf("the conflict changed the groups: %+v", got)
	}
}

func TestCLIGroups(t *testing.T) {
	s := migrated(t)
	mirror(t, s, "site-a")
	ctx := t.Context()
	jordan := Change{Actor: audit.Actor{Kind: audit.ActorOSUser, ID: "jordan"}}
	k := secrets.NewMasterKey()

	added, err := s.AddGroup(ctx, testGroup("site-b"), sealer(k, "key-1"), Change{Actor: jordan.Actor, Reason: "a new site", Check: CheckPassed})
	if err != nil {
		t.Fatal(err)
	}
	if added.Group.ManagedBy != config.ManagedByCLI || added.CreatedBy != "os-user:jordan" || added.ID == 0 {
		t.Errorf("AddGroup() = %+v", added)
	}
	if key, err := secrets.Open(k, GroupPlace(added.ID), added.APIKey); err != nil || key != "key-1" {
		t.Errorf("the stored key opens to %q, %v", key, err)
	}
	for _, name := range []string{"site-a", "site-b"} {
		if _, err := s.AddGroup(ctx, testGroup(name), sealer(k, "key"), jordan); err == nil || !strings.Contains(err.Error(), "server group "+name+" exists") {
			t.Errorf("adding %s again: %v", name, err)
		}
	}

	changed := added.Group
	changed.Views, changed.DriftPolicy = []string{"internal"}, config.PolicyIgnore
	fields, err := s.SetGroup(ctx, added, changed, nil, jordan)
	if err != nil || !reflect.DeepEqual(fields, []string{"drift_policy", "views"}) {
		t.Fatalf("SetGroup() = %v, %v", fields, err)
	}
	// prev is stale now.
	if _, err := s.SetGroup(ctx, added, changed, nil, jordan); err == nil || !strings.Contains(err.Error(), "it changed since nbpdns read it") {
		t.Errorf("SetGroup() of a stale group: %v", err)
	}
	cur, err := s.Group(ctx, "site-b")
	if err != nil {
		t.Fatal(err)
	}
	if fields, err := s.SetGroup(ctx, cur, changed, nil, jordan); err != nil || fields != nil {
		t.Errorf("SetGroup() with no change = %v, %v", fields, err)
	}
	if fields, err := s.SetGroup(ctx, cur, changed, sealer(k, "key-2"), Change{Actor: jordan.Actor, Reason: "rotated", Check: CheckSkipped}); err != nil || !reflect.DeepEqual(fields, []string{"primary.api_key"}) {
		t.Errorf("SetGroup() of the key = %v, %v", fields, err)
	}
	if v, _ := s.GroupsVersion(ctx); v != 4 {
		t.Errorf("version %d, want 4: the mirror, the add and two changes", v)
	}

	// A file group isn't the CLI's.
	var fe *FileGroupError
	site, err := s.Group(ctx, "site-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetGroup(ctx, site, changed, nil, jordan); !errors.As(err, &fe) || fe.File != "nbpdns.yaml" {
		t.Errorf("SetGroup() of a file group: %v", err)
	}
	if err := s.RemoveGroup(ctx, "site-a", jordan); !errors.As(err, &fe) {
		t.Errorf("RemoveGroup() of a file group: %v", err)
	}
	if err := s.RenameGroup(ctx, "site-a", "site-z", jordan); !errors.As(err, &fe) {
		t.Errorf("RenameGroup() of a file group: %v", err)
	}
	var ne *NoGroupError
	if _, err := s.Group(ctx, "nothing"); !errors.As(err, &ne) || !strings.Contains(err.Error(), "no server group is named nothing") {
		t.Errorf("Group() of no group: %v", err)
	}
	if err := s.RemoveGroup(ctx, "nothing", jordan); !errors.As(err, &ne) {
		t.Errorf("RemoveGroup() of no group: %v", err)
	}

	if err := s.RemoveGroup(ctx, "site-b", jordan); err != nil {
		t.Fatal(err)
	}
	events := groupEvents(t, s)[1:]
	want := []string{
		`group.created site-b os-user:jordan  {"check":"passed","drift_policy":"report","managed_by":"cli","primary.api_key":"[redacted]","primary.server_id":"localhost","primary.url":"http://pdns.example:8081","views":["_default_"]}`,
		`group.changed site-b os-user:jordan {"drift_policy":"report","views":["_default_"]} {"drift_policy":"ignore","views":["internal"]}`,
		`group.changed site-b os-user:jordan {"primary.api_key":"[redacted]"} {"check":"skipped","primary.api_key":"[redacted]"}`,
		`group.removed site-b os-user:jordan {"drift_policy":"ignore","managed_by":"cli","primary.api_key":"[redacted]","primary.server_id":"localhost","primary.url":"http://pdns.example:8081","views":["internal"]} `,
	}
	if !reflect.DeepEqual(events, want) {
		t.Errorf("events:\n%s\nwant:\n%s", strings.Join(events, "\n"), strings.Join(want, "\n"))
	}
	// The key never reaches the trail.
	all, err := s.ListAudit(ctx, AuditFilter{})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(all)
	if strings.Contains(string(b), "key-1") || strings.Contains(string(b), "key-2") {
		t.Error("the audit trail holds a key")
	}
}

func TestRenameGroup(t *testing.T) {
	s := migrated(t)
	ctx := t.Context()
	jordan := Change{Actor: audit.Actor{Kind: audit.ActorOSUser, ID: "jordan"}}
	k := secrets.NewMasterKey()
	mirror(t, s, "lab-b")
	lab, err := s.AddGroup(ctx, testGroup("lab-a"), sealer(k, "key"), jordan)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	zones := []drift.ZoneReport{zone("a.example.", drift.StateInSync), zone("b.example.", drift.StateDrift, change("192.0.2.9"))}
	if _, _, err := s.RecordRefresh(ctx, full(t0, group(zones...))); err != nil {
		t.Fatal(err)
	}

	if err := s.RenameGroup(ctx, "lab-a", "lab-b", jordan); err == nil || !strings.Contains(err.Error(), "server group lab-b exists") {
		t.Errorf("a rename to another group's name: %v", err)
	}
	if err := s.RenameGroup(ctx, "lab-a", "lab-c", jordan); err != nil {
		t.Fatal(err)
	}
	renamed, err := s.Group(ctx, "lab-c")
	if err != nil || renamed.ID != lab.ID || renamed.APIKey != lab.APIKey {
		t.Fatalf("Group(lab-c) = %+v, %v, want lab-a's ID and key", renamed, err)
	}
	// The key opens at the same place, so nothing was sealed again.
	if key, err := secrets.Open(k, GroupPlace(renamed.ID), renamed.APIKey); err != nil || key != "key" {
		t.Errorf("the key opens to %q, %v", key, err)
	}
	if _, err := s.Group(ctx, "lab-a"); err == nil {
		t.Error("lab-a is still stored")
	}
	if rows, err := s.ZoneHistory(ctx, "lab-c", "b.example.", 0, 10); err != nil || len(rows) != 1 {
		t.Errorf("lab-c's zone history = %+v, %v, want lab-a's row", rows, err)
	}
	if refreshes, err := s.Refreshes(ctx, "lab-c", 0, 10); err != nil || len(refreshes) != 1 || refreshes[0].Group == nil {
		t.Errorf("lab-c's refreshes = %+v, %v, want lab-a's result", refreshes, err)
	}
	restored, err := s.RestoreDrift(ctx)
	if _, old := restored.Groups["lab-a"]; err != nil || restored.Groups["lab-c"].Report == nil || old {
		t.Errorf("RestoreDrift() = %+v, %v, want lab-a's report as lab-c's", restored.Groups, err)
	}

	// A refresh that the rename overtook writes nothing under the old name,
	// and the next one, under the new name, finds no change.
	if rows, events, err := s.RecordRefresh(ctx, full(t0.Add(time.Hour), group(zones...))); err != nil || rows != 0 || len(events) != 0 {
		t.Errorf("a refresh under the old name wrote %d rows, %d events, %v", rows, len(events), err)
	}
	if rows, err := s.ZoneHistory(ctx, "lab-a", "b.example.", 0, 10); err != nil || len(rows) != 0 {
		t.Errorf("lab-a has rows again: %+v, %v", rows, err)
	}
	next := group(zones...)
	next.Group = "lab-c"
	if rows, events, err := s.RecordRefresh(ctx, full(t0.Add(2*time.Hour), next)); err != nil || rows != 0 || len(events) != 0 {
		t.Errorf("the first refresh after the rename wrote %d rows, %d events, %v, want none", rows, len(events), err)
	}

	// A removed group's history keeps its name until it's pruned.
	if err := s.RemoveGroup(ctx, "lab-c", jordan); err != nil {
		t.Fatal(err)
	}
	other, err := s.AddGroup(ctx, testGroup("lab-d"), sealer(k, "key"), jordan)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RenameGroup(ctx, other.Group.Name, "lab-c", jordan); err == nil ||
		!strings.Contains(err.Error(), "the drift history still holds a removed group's rows under the name lab-c") {
		t.Errorf("a rename into a removed group's history: %v", err)
	}
	events := groupEvents(t, s)
	if i := slices.IndexFunc(events, func(e string) bool { return strings.HasPrefix(e, "group.renamed") }); i < 0 ||
		events[i] != `group.renamed lab-c os-user:jordan {"name":"lab-a"} {"name":"lab-c"}` {
		t.Errorf("events = %q, want lab-a's rename", events)
	}
}

func TestGroupKeysAndTheMasterKey(t *testing.T) {
	s := migrated(t)
	ctx := t.Context()
	jordan := Change{Actor: audit.Actor{Kind: audit.ActorOSUser, ID: "jordan"}}
	k, other := secrets.NewMasterKey(), secrets.NewMasterKey()
	g, err := s.AddGroup(ctx, testGroup("site-a"), sealer(k, "key"), jordan)
	if err != nil {
		t.Fatal(err)
	}
	want := "the database's secrets are sealed with master key " + k.ID() + ", but database.master_key is master key " + other.ID()
	if _, err := s.AddGroup(ctx, testGroup("site-b"), sealer(other, "key"), jordan); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("a group's key sealed with another master key: %v", err)
	}
	sealed, err := secrets.Seal(other, SettingPlace("netbox.token"), "nbt_a.b")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeSetting(ctx, SettingChange{Key: "netbox.token", Value: sealed, Secret: true, Actor: jordan.Actor}); err == nil ||
		!strings.Contains(err.Error(), want) {
		t.Errorf("a setting sealed with another master key than a group's: %v", err)
	}
	if _, err := s.AddGroup(ctx, testGroup("site-b"), func(string) (string, error) { return "plain", nil }, jordan); err == nil ||
		!strings.Contains(err.Error(), "its API key must be sealed") {
		t.Errorf("an unsealed key: %v", err)
	}

	n, err := s.RotateMasterKey(ctx, k.ID(), other.ID(), func(place, stored string) (string, error) {
		return secrets.Rewrap(k, other, place, stored)
	}, jordan.Actor, "")
	if err != nil || n != 1 {
		t.Fatalf("RotateMasterKey() = %d, %v, want the group's key", n, err)
	}
	rotated, err := s.Group(ctx, "site-a")
	if err != nil {
		t.Fatal(err)
	}
	if key, err := secrets.Open(other, GroupPlace(g.ID), rotated.APIKey); err != nil || key != "key" {
		t.Errorf("the rotated key opens to %q, %v", key, err)
	}
}

func TestPruneRemovedGroups(t *testing.T) {
	s := migrated(t)
	mirror(t, s, "lab-a", "lab-b")
	ctx := t.Context()
	now := time.Now().UTC()
	old := now.AddDate(0, 0, -10)
	b := group(zone("b.example.", drift.StateInSync))
	b.Group = "lab-b"
	if _, _, err := s.RecordRefresh(ctx, full(old, group(zone("a.example.", drift.StateInSync)), b)); err != nil {
		t.Fatal(err)
	}
	// lab-b leaves the file; its last report goes, and its history stays
	// until the prune.
	mirror(t, s, "lab-a")
	if restored, err := s.RestoreDrift(ctx); err != nil || restored.Groups["lab-b"].Report != nil {
		t.Errorf("lab-b's report is kept: %+v, %v", restored.Groups["lab-b"], err)
	}
	if rows, err := s.ZoneHistory(ctx, "lab-b", "b.example.", 0, 10); err != nil || len(rows) != 1 {
		t.Errorf("lab-b's history = %+v, %v, want its row", rows, err)
	}
	_, rows, err := s.PruneDriftHistory(ctx, now.AddDate(0, 0, -7))
	if err != nil || rows != 1 {
		t.Errorf("PruneDriftHistory() = %d rows, %v, want lab-b's last row", rows, err)
	}
	if rows, err := s.ZoneHistory(ctx, "lab-a", "a.example.", 0, 10); err != nil || len(rows) != 1 {
		t.Errorf("lab-a's last row = %+v, %v, want it kept", rows, err)
	}
}
