package store

import (
	"encoding/json"
	"testing"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
)

func TestChangeSetting(t *testing.T) {
	s := migrated(t)
	ctx := t.Context()
	jordan := audit.Actor{Kind: audit.ActorOSUser, ID: "jordan"}
	steps := []struct {
		name    string
		c       SettingChange
		changed bool
		version int64
		stored  string // the stored value after, or empty for none
	}{
		{"set", SettingChange{Key: "drift.interval", Value: "1m", Actor: jordan, Reason: "testing"}, true, 1, "1m"},
		{"the same value", SettingChange{Key: "drift.interval", Value: "1m", Actor: jordan}, false, 1, "1m"},
		{"another value", SettingChange{Key: "drift.interval", Value: "2m", Actor: jordan}, true, 2, "2m"},
		{"unset", SettingChange{Key: "drift.interval", Unset: true, Actor: jordan}, true, 3, ""},
		{"unset again", SettingChange{Key: "drift.interval", Unset: true, Actor: jordan}, false, 3, ""},
		{"a secret", SettingChange{Key: "netbox.token", Value: "nbt_a.b", Secret: true, Actor: jordan}, true, 4, "nbt_a.b"},
		{"the same secret, written again", SettingChange{Key: "netbox.token", Value: "nbt_a.b", Secret: true, Actor: jordan}, true, 5, "nbt_a.b"},
	}
	for _, step := range steps {
		changed, err := s.ChangeSetting(ctx, step.c)
		if err != nil || changed != step.changed {
			t.Fatalf("%s: ChangeSetting() = %v, %v, want %v", step.name, changed, err, step.changed)
		}
		stored, version, err := s.Settings(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if v, _ := s.SettingsVersion(ctx); version != step.version || v != step.version {
			t.Errorf("%s: version %d and %d, want %d", step.name, version, v, step.version)
		}
		got, ok := stored[step.c.Key]
		if step.stored == "" && ok || step.stored != "" && (got.Value != step.stored || got.UpdatedBy != "os-user:jordan") {
			t.Errorf("%s: stored %+v, %v, want %q", step.name, got, ok, step.stored)
		}
	}

	events, err := s.ListAudit(ctx, AuditFilter{TargetType: "setting"})
	if err != nil {
		t.Fatal(err)
	}
	type values struct{ Value string }
	want := []struct {
		action        audit.Action
		before, after string
	}{
		{audit.SettingSet, "", "1m"},
		{audit.SettingSet, "1m", "2m"},
		{audit.SettingUnset, "2m", ""},
		{audit.SettingSet, "", "[redacted]"},
		{audit.SettingSet, "[redacted]", "[redacted]"},
	}
	if len(events) != len(want) {
		t.Fatalf("%d events, want %d: %+v", len(events), len(want), events)
	}
	for i, e := range events {
		var before, after values
		if e.Before != nil {
			_ = json.Unmarshal(e.Before, &before)
		}
		if e.After != nil {
			_ = json.Unmarshal(e.After, &after)
		}
		if e.Action != want[i].action || before.Value != want[i].before || after.Value != want[i].after || e.Actor != jordan {
			t.Errorf("event %d = %s %s -> %s by %s, want %s %q -> %q", i, e.Action, e.Before, e.After, e.Actor, want[i].action, want[i].before, want[i].after)
		}
	}
	if events[0].Reason != "testing" {
		t.Errorf("the first event's reason = %q", events[0].Reason)
	}
}
