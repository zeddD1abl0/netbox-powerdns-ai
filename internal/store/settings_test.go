package store

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/secrets"
)

func TestChangeSetting(t *testing.T) {
	onEach(t, func(t *testing.T, db backend) {
		s := db.migrated(t)
		ctx := t.Context()
		jordan := audit.Actor{Kind: audit.ActorOSUser, ID: "jordan"}
		sealed, err := secrets.Seal(secrets.NewMasterKey(), SettingPlace("netbox.token"), "nbt_a.b")
		if err != nil {
			t.Fatal(err)
		}
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
			{"a secret", SettingChange{Key: "netbox.token", Value: sealed, Secret: true, Actor: jordan}, true, 4, sealed},
			{"the same secret, written again", SettingChange{Key: "netbox.token", Value: sealed, Secret: true, Actor: jordan}, true, 5, sealed},
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
		if _, err := s.ChangeSetting(ctx, SettingChange{Key: "netbox.token", Value: "nbt_a.b", Secret: true, Actor: jordan}); err == nil {
			t.Error("ChangeSetting() stored an unsealed secret")
		}
	})
}

// TestRotateMasterKey re-wraps the sealed settings with a new master key,
// and refuses, changing nothing, when one can't be.
func TestRotateMasterKey(t *testing.T) {
	onEach(t, func(t *testing.T, db backend) {
		s := db.migrated(t)
		ctx := t.Context()
		old, next := secrets.NewMasterKey(), secrets.NewMasterKey()
		jordan := audit.Actor{Kind: audit.ActorOSUser, ID: "jordan"}
		for key, value := range map[string]string{"netbox.token": "nbt_a.b", "netbox.webhook_secret": "secret-secret-secret"} {
			sealed, err := secrets.Seal(old, SettingPlace(key), value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.ChangeSetting(ctx, SettingChange{Key: key, Value: sealed, Secret: true, Actor: jordan}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.ChangeSetting(ctx, SettingChange{Key: "drift.interval", Value: "1m", Actor: jordan}); err != nil {
			t.Fatal(err)
		}
		// The wrong old key re-wraps nothing.
		wrong := secrets.NewMasterKey()
		if _, err := s.RotateMasterKey(ctx, wrong.ID(), next.ID(), func(place, stored string) (string, error) {
			return secrets.Rewrap(wrong, next, place, stored)
		}, jordan, ""); err == nil {
			t.Fatal("RotateMasterKey() with the wrong key succeeded")
		}
		n, err := s.RotateMasterKey(ctx, old.ID(), next.ID(), func(place, stored string) (string, error) {
			return secrets.Rewrap(old, next, place, stored)
		}, jordan, "yearly")
		if err != nil || n != 2 {
			t.Fatalf("RotateMasterKey() = %d, %v, want 2", n, err)
		}
		// A secret sealed with the old key, as by a CLI that sealed it while the
		// rotation ran, is refused, since serve couldn't open it with the new.
		late, err := secrets.Seal(old, SettingPlace("netbox.token"), "nbt_c.d")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.ChangeSetting(ctx, SettingChange{Key: "netbox.token", Value: late, Secret: true, Actor: jordan}); err == nil ||
			!strings.Contains(err.Error(), "the database's secrets are sealed with master key "+next.ID()) {
			t.Errorf("ChangeSetting() with the old key after the rotation = %v", err)
		}
		stored, _, err := s.Settings(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if v, err := secrets.Open(next, SettingPlace("netbox.token"), stored["netbox.token"].Value); err != nil || v != "nbt_a.b" {
			t.Errorf("the token after the rotation = %q, %v", v, err)
		}
		if stored["drift.interval"].Value != "1m" {
			t.Errorf("drift.interval after the rotation = %+v", stored["drift.interval"])
		}
		events, err := s.ListAudit(ctx, AuditFilter{Action: audit.MasterKeyRotated})
		if err != nil || len(events) != 1 || events[0].Target.ID != next.ID() || events[0].Reason != "yearly" {
			t.Errorf("rotation events = %+v, %v", events, err)
		}
		// Once no secret is stored, as after a lost key's secrets are unset,
		// any master key seals the first again.
		for _, key := range []string{"netbox.token", "netbox.webhook_secret"} {
			if _, err := s.ChangeSetting(ctx, SettingChange{Key: key, Unset: true, Secret: true, Actor: jordan}); err != nil {
				t.Fatal(err)
			}
		}
		fresh, err := secrets.Seal(wrong, SettingPlace("netbox.token"), "nbt_e.f")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.ChangeSetting(ctx, SettingChange{Key: "netbox.token", Value: fresh, Secret: true, Actor: jordan}); err != nil {
			t.Errorf("ChangeSetting() with a new key and no secrets stored = %v", err)
		}
	})
}
