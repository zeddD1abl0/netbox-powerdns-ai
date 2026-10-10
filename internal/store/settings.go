package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store/sqlite"
)

// seqSettings is the settings' version, which each change bumps.
const seqSettings = "settings_version"

// A StoredSetting is a runtime setting's value, as the database stores it
// (ADR-0040).
type StoredSetting struct {
	Value     string
	UpdatedAt time.Time
	// UpdatedBy is the actor who last set it, such as os-user:jordan.
	UpdatedBy string
}

// Settings returns the stored settings, by key, and their version, in one
// read transaction.
func (s *Store) Settings(ctx context.Context) (map[string]StoredSetting, int64, error) {
	sqlTx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, 0, fmt.Errorf("reading the settings: %w", err)
	}
	defer func() { _ = sqlTx.Rollback() }()
	q := s.q.WithTx(sqlTx)
	rows, err := q.ListSettings(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("reading the settings: %w", err)
	}
	version, err := q.SequenceValue(ctx, seqSettings)
	if err != nil {
		return nil, 0, fmt.Errorf("reading the settings' version: %w", err)
	}
	out := make(map[string]StoredSetting, len(rows))
	for _, r := range rows {
		at, err := time.Parse(TimeLayout, r.UpdatedAt)
		if err != nil {
			return nil, 0, fmt.Errorf("reading the setting %s: %w", r.Key, err)
		}
		out[r.Key] = StoredSetting{Value: r.Value, UpdatedAt: at, UpdatedBy: r.UpdatedBy}
	}
	return out, version, nil
}

// SettingsVersion returns the settings' version, which each change bumps, so
// that serve can tell, cheaply, when to read them again.
func (s *Store) SettingsVersion(ctx context.Context) (int64, error) {
	v, err := s.q.SequenceValue(ctx, seqSettings)
	if err != nil {
		return 0, fmt.Errorf("reading the settings' version: %w", err)
	}
	return v, nil
}

// A SettingChange sets a runtime setting's stored value, or unsets it.
type SettingChange struct {
	Key   string
	Value string
	// Unset removes the stored value, rather than setting Value.
	Unset bool
	// Secret redacts the values in the change's audit event.
	Secret bool
	Actor  audit.Actor
	Reason string
}

// ChangeSetting makes c, bumps the settings' version, and records it as a
// setting.set or setting.unset event, with ctx's trace and request IDs, all
// in one transaction. It reports false, and changes nothing, if the setting
// already has the value, or has none to unset. A secret's value is always
// written, since its events can't say whether it changed.
func (s *Store) ChangeSetting(ctx context.Context, c SettingChange) (bool, error) {
	changed := false
	_, err := s.InTx(ctx, func(tx *Tx) error {
		old, err := tx.q.GetSetting(ctx, c.Key)
		had := err == nil
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		switch {
		case c.Unset && !had, !c.Unset && had && !c.Secret && old.Value == c.Value:
			return nil
		case c.Unset:
			if _, err := tx.q.DeleteSetting(ctx, c.Key); err != nil {
				return err
			}
		default:
			if err := tx.q.UpsertSetting(ctx, sqlite.UpsertSettingParams{
				Key: c.Key, Value: c.Value, UpdatedAt: time.Now().UTC().Format(TimeLayout), UpdatedBy: c.Actor.String(),
			}); err != nil {
				return err
			}
		}
		if _, err := tx.q.NextIDs(ctx, sqlite.NextIDsParams{N: 1, Name: seqSettings}); err != nil {
			return err
		}
		shown := func(v string) (json.RawMessage, error) {
			if c.Secret {
				v = config.Redacted
			}
			return audit.Value(map[string]string{"value": v})
		}
		e := audit.Event{Actor: c.Actor, Action: audit.SettingSet, Target: audit.Target{Type: "setting", ID: c.Key}, Reason: c.Reason}
		if had {
			if e.Before, err = shown(old.Value); err != nil {
				return err
			}
		}
		if c.Unset {
			e.Action = audit.SettingUnset
		} else if e.After, err = shown(c.Value); err != nil {
			return err
		}
		if _, err := tx.Audit(ctx, e.WithContext(ctx)); err != nil {
			return err
		}
		changed = true
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("changing the setting %s: %w", c.Key, err)
	}
	return changed, nil
}
