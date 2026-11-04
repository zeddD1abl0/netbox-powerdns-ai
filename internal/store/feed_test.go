package store

import (
	"testing"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
)

// TestVersions checks that the change feed's versions move with each change
// to the settings and to the groups, on either database, and that no
// replica leads a new one (ADR-0048).
func TestVersions(t *testing.T) {
	onEach(t, func(t *testing.T, db backend) {
		s := db.migrated(t)
		ctx := t.Context()
		if v, err := s.Versions(ctx); err != nil || v != (Versions{}) {
			t.Fatalf("Versions() of a new database = %+v, %v, want zeros", v, err)
		}
		jordan := audit.Actor{Kind: audit.ActorOSUser, ID: "jordan"}
		if _, err := s.ChangeSetting(ctx, SettingChange{Key: "drift.interval", Value: "1m", Actor: jordan}); err != nil {
			t.Fatal(err)
		}
		if v, err := s.Versions(ctx); err != nil || v != (Versions{Settings: 1}) {
			t.Errorf("Versions() after a setting = %+v, %v, want the settings' 1", v, err)
		}
		if _, err := s.MirrorFileGroups(ctx, []config.Group{testGroup("site-a")}, audit.File("nbpdns.yaml")); err != nil {
			t.Fatal(err)
		}
		if v, err := s.Versions(ctx); err != nil || v != (Versions{Settings: 1, Groups: 1}) {
			t.Errorf("Versions() after a group = %+v, %v, want 1 and 1", v, err)
		}
	})
}

// TestListenOnSQLite checks that only PostgreSQL listens.
func TestListenOnSQLite(t *testing.T) {
	s := sqliteBackend.migrated(t)
	if l, err := s.Listen(t.Context()); err == nil || err.Error() != "listening for the database's changes: only PostgreSQL has LISTEN and NOTIFY" {
		t.Errorf("Listen() on SQLite = %v, %v, want a refusal", l, err)
	}
}
