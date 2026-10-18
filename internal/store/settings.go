package store

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/secrets"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store/sqlite"
)

// seqSettings is the settings' version, which each change bumps.
const seqSettings = "settings_version"

// A StoredSetting is a runtime setting's value, as the database stores it
// (ADR-0040).
type StoredSetting struct {
	// Value is the value, or, if Sealed, its sealed form, which the master
	// key opens (ADR-0041).
	Value     string
	Sealed    bool
	UpdatedAt time.Time
	// UpdatedBy is the actor who last set it, such as os-user:jordan.
	UpdatedBy string
}

// Settings returns the stored settings, by key, and their version, in one
// read transaction.
func (s *Store) Settings(ctx context.Context) (map[string]StoredSetting, int64, error) {
	sqlTx, q, err := s.readTx(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("reading the settings: %w", err)
	}
	defer func() { _ = sqlTx.Rollback() }()
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
		out[r.Key] = StoredSetting{Value: r.Value, Sealed: r.Sealed, UpdatedAt: at, UpdatedBy: r.UpdatedBy}
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

// SettingPlace is where a setting's sealed value belongs, which the seal
// binds it to.
func SettingPlace(key string) string { return "setting:" + key }

// A SettingChange sets a runtime setting's stored value, or unsets it.
type SettingChange struct {
	Key string
	// Value is the value to store: for a secret, its sealed form.
	Value string
	// Unset removes the stored value, rather than setting Value.
	Unset bool
	// Secret marks a sealed Value, and redacts the values in the change's
	// audit event. No secret is stored unsealed.
	Secret bool
	Actor  audit.Actor
	Reason string
}

// ChangeSetting makes c, bumps the settings' version, and records it as a
// setting.set or setting.unset event, with ctx's trace and request IDs, all
// in one transaction. It reports false, and changes nothing, if the setting
// already has the value, or has none to unset. A secret's value is always
// written, since its events can't say whether it changed. A secret sealed
// with another master key than the database's other secrets is refused.
func (s *Store) ChangeSetting(ctx context.Context, c SettingChange) (bool, error) {
	var keyID string
	if c.Secret && !c.Unset {
		var err error
		if keyID, err = secrets.KeyID(c.Value); err != nil {
			return false, fmt.Errorf("changing the setting %s: a secret must be sealed: %w", c.Key, err)
		}
	}
	changed := false
	_, err := s.InTx(ctx, func(tx *Tx) error {
		if keyID != "" {
			if err := tx.checkMasterKey(ctx, keyID); err != nil {
				return err
			}
		}
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
				Key: c.Key, Value: c.Value, Sealed: c.Secret, UpdatedAt: time.Now().UTC().Format(TimeLayout), UpdatedBy: c.Actor.String(),
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

// checkMasterKey refuses a value sealed with the master key keyID if the
// database stores a value sealed with another, the one it replaces
// included, since serve opens them all with one key (ADR-0041). Such a
// value comes from a master key that was rotated away, perhaps while it was
// sealed. The transaction holds the write lock, so a rotation can't come
// between the check and the write.
func (tx *Tx) checkMasterKey(ctx context.Context, keyID string) error {
	sealed, err := tx.sealed(ctx)
	if err != nil {
		return err
	}
	for _, v := range sealed {
		id, err := secrets.KeyID(v.value)
		if err != nil {
			return fmt.Errorf("%s: %w", v.place, err)
		}
		if id != keyID {
			return fmt.Errorf("the database's secrets are sealed with master key %s, but database.master_key is master key %s; "+
				"if the master key was rotated, set database.master_key to the new key", id, keyID)
		}
	}
	return nil
}

// A sealedValue is a value that the database stores sealed, at its place,
// with the function that replaces it.
type sealedValue struct {
	place, value string
	update       func(ctx context.Context, value string) error
}

// sealed returns every value that the database stores sealed: the secret
// settings', and the CLI groups' API keys.
func (tx *Tx) sealed(ctx context.Context) ([]sealedValue, error) {
	settings, err := tx.q.SealedSettings(ctx)
	if err != nil {
		return nil, err
	}
	// By key, byte by byte, on either database (ADR-0046).
	slices.SortFunc(settings, func(a, b sqlite.SealedSettingsRow) int { return cmp.Compare(a.Key, b.Key) })
	groups, err := tx.q.SealedGroupKeys(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]sealedValue, 0, len(settings)+len(groups))
	for _, r := range settings {
		out = append(out, sealedValue{place: SettingPlace(r.Key), value: r.Value, update: func(ctx context.Context, value string) error {
			return tx.q.UpdateSealedSetting(ctx, sqlite.UpdateSealedSettingParams{Key: r.Key, Value: value})
		}})
	}
	for _, r := range groups {
		out = append(out, sealedValue{place: GroupPlace(r.ID), value: r.ApiKey, update: func(ctx context.Context, value string) error {
			return tx.q.UpdateGroupKey(ctx, sqlite.UpdateGroupKeyParams{ApiKey: value, ID: r.ID})
		}})
	}
	return out, nil
}

// A Rewrap returns a value sealed for place, stored, with its data key
// wrapped by the new master key.
type Rewrap func(place, stored string) (string, error)

// RotateMasterKey re-wraps every sealed value that the database stores, the
// secret settings and the CLI groups' API keys, with rewrap, from the
// master key oldID to newID, and records it as a master_key.rotated event,
// in one transaction (ADR-0041). It returns how many values it re-wrapped.
// If any fails, it changes nothing.
func (s *Store) RotateMasterKey(ctx context.Context, oldID, newID string, rewrap Rewrap, actor audit.Actor, reason string) (int, error) {
	n := 0
	_, err := s.InTx(ctx, func(tx *Tx) error {
		sealed, err := tx.sealed(ctx)
		if err != nil {
			return err
		}
		for _, v := range sealed {
			value, err := rewrap(v.place, v.value)
			if err != nil {
				return fmt.Errorf("%s: %w", v.place, err)
			}
			if err := v.update(ctx, value); err != nil {
				return err
			}
			n++
		}
		before, err := audit.Value(map[string]string{"key_id": oldID})
		if err != nil {
			return err
		}
		after, err := audit.Value(map[string]any{"key_id": newID, "secrets": n})
		if err != nil {
			return err
		}
		_, err = tx.Audit(ctx, audit.Event{Actor: actor, Action: audit.MasterKeyRotated, Target: audit.Target{Type: "master-key", ID: newID},
			Before: before, After: after, Reason: reason}.WithContext(ctx))
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("rotating the master key: %w", err)
	}
	return n, nil
}
