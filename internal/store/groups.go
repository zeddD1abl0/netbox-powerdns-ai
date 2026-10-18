package store

import (
	"bytes"
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/config"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/secrets"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store/sqlite"
)

// The sequences of the server groups: their IDs, and their version, which
// each change bumps.
const (
	seqGroups        = "server_groups"
	seqGroupsVersion = "groups_version"
)

// targetGroup is the type of a group's audit events' target.
const targetGroup = "server-group"

// A StoredGroup is a server group, as the database stores it (ADR-0040).
type StoredGroup struct {
	// ID is the group's, which nbpdns assigns and never reuses, and which a
	// rename keeps (ADR-0045).
	ID int64
	// Group is the group, with its ManagedBy, but without its API key.
	Group config.Group
	// APIKey is a CLI group's API key, sealed for GroupPlace(ID)
	// (ADR-0041), or empty for a file group, whose key stays in the file.
	APIKey string
	// CreatedBy and UpdatedBy are the actors who added it and last changed
	// it, such as os-user:jordan, or file:/etc/nbpdns/nbpdns.yaml.
	CreatedAt, UpdatedAt time.Time
	CreatedBy, UpdatedBy string
}

// GroupPlace is where the sealed API key of the group whose ID is id
// belongs, which the seal binds it to. It's the group's ID, not its name, so
// that a rename re-seals nothing (ADR-0045).
func GroupPlace(id int64) string { return "server-group:" + strconv.FormatInt(id, 10) }

// A Seal seals a group's API key for place, its GroupPlace.
type Seal func(place string) (string, error)

// How a change to a group checked its primary: see Change.Check.
const (
	CheckPassed  = "passed"
	CheckSkipped = "skipped"
)

// A Change says who changes a group, and why, for its audit event.
type Change struct {
	Actor  audit.Actor
	Reason string
	// Check is how the change checked the group's primary: CheckPassed,
	// CheckSkipped with --no-check, or empty if it didn't, since it didn't
	// touch how nbpdns reaches it. The event's after shows it as check.
	Check string
}

// NoGroupError is the error for a group that the database doesn't store.
type NoGroupError struct{ Name string }

func (e *NoGroupError) Error() string {
	return fmt.Sprintf("no server group is named %s; nbpdns groups list lists them", e.Name)
}

// FileGroupError is the error for a change to a file-managed group, which
// only the config file changes.
type FileGroupError struct {
	Name string
	// File is the config file that it was last mirrored from.
	File string
}

func (e *FileGroupError) Error() string {
	return fmt.Sprintf("server group %s is managed by the config file, %s; change it there", e.Name, e.File)
}

// ConflictError is the error for a group in the config file that has the
// name of a group that `nbpdns groups add` added, which stops serve
// (ADR-0040).
type ConflictError struct {
	Name string
	// AddedBy is the actor who added the CLI's group.
	AddedBy string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("server group %s is in the config file, and %s added a group of that name with nbpdns groups add; "+
		"remove it from the config file, or with nbpdns groups remove", e.Name, e.AddedBy)
}

// groupSpec is a group's fields but its name and API key, as spec_json
// holds them, named as the config file names them.
type groupSpec struct {
	Views        []string          `json:"views"`
	DriftPolicy  string            `json:"drift_policy"`
	ZonePolicies map[string]string `json:"zone_policies,omitempty"`
	URL          string            `json:"primary.url"`
	ServerID     string            `json:"primary.server_id"`
	APIKeyFile   string            `json:"primary.api_key_file,omitempty"`
	CAFile       string            `json:"primary.ca_file,omitempty"`
	CertFile     string            `json:"primary.cert_file,omitempty"`
	KeyFile      string            `json:"primary.key_file,omitempty"`
}

// encodeSpec returns g's spec_json. It's the same for the same fields, so
// that comparing two says whether they changed.
func encodeSpec(g config.Group) (string, error) {
	p := g.Primary
	return jsonText(groupSpec{
		Views: nonNil(g.Views), DriftPolicy: g.DriftPolicy, ZonePolicies: g.ZonePolicies, URL: p.URL, ServerID: p.ServerID,
		APIKeyFile: p.APIKeyFile, CAFile: p.CAFile, CertFile: p.CertFile, KeyFile: p.KeyFile,
	})
}

// storedGroup decodes a group's row.
func storedGroup(r sqlite.ServerGroup) (StoredGroup, error) {
	var sp groupSpec
	if err := json.Unmarshal([]byte(r.SpecJson), &sp); err != nil {
		return StoredGroup{}, fmt.Errorf("reading server group %s: %w", r.Name, err)
	}
	out := StoredGroup{
		ID: r.ID, APIKey: r.ApiKey, CreatedBy: r.CreatedBy, UpdatedBy: r.UpdatedBy,
		Group: config.Group{
			Name: r.Name, ManagedBy: r.ManagedBy, Views: sp.Views, DriftPolicy: sp.DriftPolicy, ZonePolicies: sp.ZonePolicies,
			Primary: config.Primary{
				URL: sp.URL, ServerID: sp.ServerID, APIKeyFile: sp.APIKeyFile, CAFile: sp.CAFile, CertFile: sp.CertFile, KeyFile: sp.KeyFile,
			},
		},
	}
	var err error
	if out.CreatedAt, err = time.Parse(TimeLayout, r.CreatedAt); err != nil {
		return StoredGroup{}, fmt.Errorf("reading server group %s: %w", r.Name, err)
	}
	if out.UpdatedAt, err = time.Parse(TimeLayout, r.UpdatedAt); err != nil {
		return StoredGroup{}, fmt.Errorf("reading server group %s: %w", r.Name, err)
	}
	return out, nil
}

// Groups returns the groups that the database stores, by name, and their
// version, in one read transaction.
func (s *Store) Groups(ctx context.Context) ([]StoredGroup, int64, error) {
	sqlTx, q, err := s.readTx(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("reading the server groups: %w", err)
	}
	defer func() { _ = sqlTx.Rollback() }()
	rows, err := listGroups(ctx, q)
	if err != nil {
		return nil, 0, fmt.Errorf("reading the server groups: %w", err)
	}
	version, err := q.SequenceValue(ctx, seqGroupsVersion)
	if err != nil {
		return nil, 0, fmt.Errorf("reading the server groups' version: %w", err)
	}
	out := make([]StoredGroup, len(rows))
	for i, r := range rows {
		if out[i], err = storedGroup(r); err != nil {
			return nil, 0, err
		}
	}
	return out, version, nil
}

// listGroups returns the stored groups by name, sorted byte by byte, so
// that they come in one order on either database, whatever its collation
// (ADR-0046).
func listGroups(ctx context.Context, q sqlite.Querier) ([]sqlite.ServerGroup, error) {
	rows, err := q.ListGroups(ctx)
	slices.SortFunc(rows, func(a, b sqlite.ServerGroup) int { return cmp.Compare(a.Name, b.Name) })
	return rows, err
}

// GroupsVersion returns the groups' version, which each change bumps, so
// that serve can tell, cheaply, when to read them again.
func (s *Store) GroupsVersion(ctx context.Context) (int64, error) {
	v, err := s.q.SequenceValue(ctx, seqGroupsVersion)
	if err != nil {
		return 0, fmt.Errorf("reading the server groups' version: %w", err)
	}
	return v, nil
}

// Group returns the group named name, or a *NoGroupError.
func (s *Store) Group(ctx context.Context, name string) (StoredGroup, error) {
	r, err := s.q.GetGroup(ctx, name)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return StoredGroup{}, &NoGroupError{Name: name}
	case err != nil:
		return StoredGroup{}, fmt.Errorf("reading server group %s: %w", name, err)
	}
	return storedGroup(r)
}

// MirrorFileGroups makes the database's file-managed groups the config
// file's, groups, as serve does at each start (ADR-0040): it adds those that
// are new, changes those that changed, and removes those that the file no
// longer declares, each as an event by actor, the file, and bumps the
// groups' version if anything changed, all in one transaction. A file
// group's API key isn't stored. A group in the file that has a CLI group's
// name is a *ConflictError, and changes nothing.
func (s *Store) MirrorFileGroups(ctx context.Context, groups []config.Group, actor audit.Actor) ([]audit.Event, error) {
	events, err := s.InTx(ctx, func(tx *Tx) error {
		rows, err := listGroups(ctx, tx.q)
		if err != nil {
			return err
		}
		stored := map[string]sqlite.ServerGroup{}
		for _, r := range rows {
			stored[r.Name] = r
		}
		var conflicts []error
		for _, g := range groups {
			if r, ok := stored[g.Name]; ok && r.ManagedBy != config.ManagedByFile {
				conflicts = append(conflicts, &ConflictError{Name: g.Name, AddedBy: r.CreatedBy})
			}
		}
		if len(conflicts) > 0 {
			return errors.Join(conflicts...)
		}
		now := time.Now()
		changed := false
		declared := map[string]bool{}
		for _, g := range groups {
			declared[g.Name] = true
			g.ManagedBy = config.ManagedByFile
			r, ok := stored[g.Name]
			if !ok {
				if _, err := tx.addGroup(ctx, g, nil, Change{Actor: actor}, now); err != nil {
					return err
				}
				changed = true
				continue
			}
			fields, err := tx.changeGroup(ctx, r, g, nil, Change{Actor: actor}, now)
			if err != nil {
				return err
			}
			changed = changed || len(fields) > 0
		}
		for _, r := range rows {
			if r.ManagedBy != config.ManagedByFile || declared[r.Name] {
				continue
			}
			if err := tx.removeGroup(ctx, r, Change{Actor: actor}, now); err != nil {
				return err
			}
			changed = true
		}
		if changed {
			return tx.bumpGroups(ctx)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("mirroring the config file's server groups: %w", err)
	}
	return events, nil
}

// AddGroup adds g, a CLI group, with its API key as seal seals it for the
// group's place, records it as a group.created event by c, and bumps the
// groups' version, all in one transaction. It fails if a group has g's
// name, or if the key is sealed with another master key than the
// database's other secrets.
func (s *Store) AddGroup(ctx context.Context, g config.Group, seal Seal, c Change) (StoredGroup, error) {
	var out StoredGroup
	_, err := s.InTx(ctx, func(tx *Tx) error {
		switch r, err := tx.q.GetGroup(ctx, g.Name); {
		case err == nil:
			return fmt.Errorf("server group %s exists, managed by %s", g.Name, r.ManagedBy)
		case !errors.Is(err, sql.ErrNoRows):
			return err
		}
		g.ManagedBy = config.ManagedByCLI
		r, err := tx.addGroup(ctx, g, seal, c, time.Now())
		if err != nil {
			return err
		}
		if out, err = storedGroup(r); err != nil {
			return err
		}
		return tx.bumpGroups(ctx)
	})
	if err != nil {
		return StoredGroup{}, fmt.Errorf("adding server group %s: %w", g.Name, err)
	}
	return out, nil
}

// SetGroup changes prev, a CLI group as Group or Groups read it, to g, and,
// if seal isn't nil, its API key to what seal seals for its place; records
// the fields that changed as a group.changed event by c; and bumps the
// groups' version, all in one transaction. It returns the names of the
// fields that changed, and changes nothing if none did. It fails if the
// group changed since prev was read.
func (s *Store) SetGroup(ctx context.Context, prev StoredGroup, g config.Group, seal Seal, c Change) ([]string, error) {
	var fields []string
	_, err := s.InTx(ctx, func(tx *Tx) error {
		r, err := tx.cliGroup(ctx, prev.Group.Name)
		if err != nil {
			return err
		}
		spec, err := encodeSpec(prev.Group)
		if err != nil {
			return err
		}
		if r.ID != prev.ID || r.SpecJson != spec || r.ApiKey != prev.APIKey {
			return errors.New("it changed since nbpdns read it; try again")
		}
		g.Name, g.ManagedBy = r.Name, r.ManagedBy
		if fields, err = tx.changeGroup(ctx, r, g, seal, c, time.Now()); err != nil || len(fields) == 0 {
			return err
		}
		return tx.bumpGroups(ctx)
	})
	if err != nil {
		return nil, fmt.Errorf("changing server group %s: %w", prev.Group.Name, err)
	}
	return fields, nil
}

// RemoveGroup removes name, a CLI group, with its last report, but keeps its
// drift history, which drift.history_retention_days prunes; records it as a
// group.removed event by c; and bumps the groups' version, all in one
// transaction.
func (s *Store) RemoveGroup(ctx context.Context, name string, c Change) error {
	_, err := s.InTx(ctx, func(tx *Tx) error {
		r, err := tx.cliGroup(ctx, name)
		if err != nil {
			return err
		}
		if err := tx.removeGroup(ctx, r, c, time.Now()); err != nil {
			return err
		}
		return tx.bumpGroups(ctx)
	})
	if err != nil {
		return fmt.Errorf("removing server group %s: %w", name, err)
	}
	return nil
}

// RenameGroup renames old, a CLI group, to name, and its drift history and
// last report with it, so that they carry on under its new name; records it
// as a group.renamed event by c; and bumps the groups' version, all in one
// transaction (ADR-0045). The audit trail keeps the old name. It fails if a
// group has the new name, or the drift history still has a removed group's
// rows under it, since the two histories would merge.
func (s *Store) RenameGroup(ctx context.Context, old, name string, c Change) error {
	_, err := s.InTx(ctx, func(tx *Tx) error {
		r, err := tx.cliGroup(ctx, old)
		if err != nil {
			return err
		}
		switch o, err := tx.q.GetGroup(ctx, name); {
		case err == nil:
			return fmt.Errorf("server group %s exists, managed by %s", name, o.ManagedBy)
		case !errors.Is(err, sql.ErrNoRows):
			return err
		}
		refreshes, err := tx.q.CountRefreshGroupRows(ctx, name)
		if err != nil {
			return err
		}
		zones, err := tx.q.CountZoneRows(ctx, name)
		if err != nil {
			return err
		}
		if refreshes+zones > 0 {
			return fmt.Errorf("the drift history still holds a removed group's rows under the name %s, until drift.history_retention_days prunes them; "+
				"choose another name", name)
		}
		now := time.Now().UTC().Format(TimeLayout)
		if err := tx.q.RenameGroup(ctx, sqlite.RenameGroupParams{NewName: name, UpdatedAt: now, UpdatedBy: c.Actor.String(), ID: r.ID}); err != nil {
			return err
		}
		// A last report under the new name outlived its group, from before
		// groups were stored.
		if err := tx.q.DeleteGroupReport(ctx, name); err != nil {
			return err
		}
		if err := tx.q.RenameRefreshGroupRows(ctx, sqlite.RenameRefreshGroupRowsParams{NewName: name, OldName: old}); err != nil {
			return err
		}
		if err := tx.q.RenameZoneRows(ctx, sqlite.RenameZoneRowsParams{NewName: name, OldName: old}); err != nil {
			return err
		}
		if err := tx.q.RenameGroupReport(ctx, sqlite.RenameGroupReportParams{NewName: name, OldName: old}); err != nil {
			return err
		}
		before, err := audit.Value(map[string]string{"name": old})
		if err != nil {
			return err
		}
		after, err := audit.Value(map[string]string{"name": name})
		if err != nil {
			return err
		}
		if _, err := tx.Audit(ctx, audit.Event{Actor: c.Actor, Action: audit.GroupRenamed, Target: audit.Target{Type: targetGroup, ID: name},
			Before: before, After: after, Reason: c.Reason}.WithContext(ctx)); err != nil {
			return err
		}
		return tx.bumpGroups(ctx)
	})
	if err != nil {
		return fmt.Errorf("renaming server group %s: %w", old, err)
	}
	return nil
}

// cliGroup returns the CLI group named name, or a *NoGroupError, or a
// *FileGroupError for a file group.
func (tx *Tx) cliGroup(ctx context.Context, name string) (sqlite.ServerGroup, error) {
	r, err := tx.q.GetGroup(ctx, name)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return r, &NoGroupError{Name: name}
	case err != nil:
		return r, err
	case r.ManagedBy == config.ManagedByFile:
		return r, &FileGroupError{Name: name, File: audit.ParseActor(r.UpdatedBy).ID}
	}
	return r, nil
}

// addGroup inserts g, with its API key as seal seals it, if seal isn't nil,
// and records it as a group.created event by c, at now. A last report under
// its name, which outlived a group from before groups were stored, is
// deleted, so that it isn't restored as g's.
func (tx *Tx) addGroup(ctx context.Context, g config.Group, seal Seal, c Change, now time.Time) (sqlite.ServerGroup, error) {
	id, err := tx.q.NextIDs(ctx, sqlite.NextIDsParams{N: 1, Name: seqGroups})
	if err != nil {
		return sqlite.ServerGroup{}, err
	}
	key, err := tx.seal(ctx, id, seal)
	if err != nil {
		return sqlite.ServerGroup{}, err
	}
	spec, err := encodeSpec(g)
	if err != nil {
		return sqlite.ServerGroup{}, err
	}
	at, by := now.UTC().Format(TimeLayout), c.Actor.String()
	r := sqlite.ServerGroup{ID: id, Name: g.Name, ManagedBy: g.ManagedBy, SpecJson: spec, ApiKey: key,
		CreatedAt: at, CreatedBy: by, UpdatedAt: at, UpdatedBy: by}
	if err := tx.q.InsertGroup(ctx, sqlite.InsertGroupParams(r)); err != nil {
		return r, err
	}
	if err := tx.q.DeleteGroupReport(ctx, g.Name); err != nil {
		return r, err
	}
	fields, err := groupFields(r)
	if err != nil {
		return r, err
	}
	after, err := pick(fields, slices.Collect(maps.Keys(fields)), c.Check)
	if err != nil {
		return r, err
	}
	_, err = tx.Audit(ctx, audit.Event{Time: now, Actor: c.Actor, Action: audit.GroupCreated, Target: audit.Target{Type: targetGroup, ID: g.Name},
		After: after, Reason: c.Reason}.WithContext(ctx))
	return r, err
}

// changeGroup changes r's fields to g's, and its API key to what seal seals,
// if seal isn't nil, and records the fields that changed as a group.changed
// event by c, at now. It returns their names, and changes nothing if none
// did.
func (tx *Tx) changeGroup(ctx context.Context, r sqlite.ServerGroup, g config.Group, seal Seal, c Change, now time.Time) ([]string, error) {
	spec, err := encodeSpec(g)
	if err != nil {
		return nil, err
	}
	next := r
	next.SpecJson = spec
	if seal != nil {
		if next.ApiKey, err = tx.seal(ctx, r.ID, seal); err != nil {
			return nil, err
		}
	}
	before, err := groupFields(r)
	if err != nil {
		return nil, err
	}
	after, err := groupFields(next)
	if err != nil {
		return nil, err
	}
	keys := slices.Collect(maps.Keys(before))
	for k := range after {
		if _, ok := before[k]; !ok {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	var changed []string
	for _, k := range keys {
		// A sealed key shows as [redacted] before and after, so it changed
		// if it was sealed again.
		if !bytes.Equal(before[k], after[k]) || k == "primary.api_key" && seal != nil {
			changed = append(changed, k)
		}
	}
	if len(changed) == 0 {
		return nil, nil
	}
	next.UpdatedAt, next.UpdatedBy = now.UTC().Format(TimeLayout), c.Actor.String()
	if err := tx.q.UpdateGroup(ctx, sqlite.UpdateGroupParams{SpecJson: next.SpecJson, ApiKey: next.ApiKey, UpdatedAt: next.UpdatedAt,
		UpdatedBy: next.UpdatedBy, ID: r.ID}); err != nil {
		return nil, err
	}
	e := audit.Event{Time: now, Actor: c.Actor, Action: audit.GroupChanged, Target: audit.Target{Type: targetGroup, ID: r.Name}, Reason: c.Reason}
	if e.Before, err = pick(before, changed, ""); err != nil {
		return nil, err
	}
	if e.After, err = pick(after, changed, c.Check); err != nil {
		return nil, err
	}
	if _, err := tx.Audit(ctx, e.WithContext(ctx)); err != nil {
		return nil, err
	}
	return changed, nil
}

// removeGroup deletes r, and its last report, and records it as a
// group.removed event by c, at now. Its drift history stays, for
// drift.history_retention_days.
func (tx *Tx) removeGroup(ctx context.Context, r sqlite.ServerGroup, c Change, now time.Time) error {
	if err := tx.q.DeleteGroup(ctx, r.ID); err != nil {
		return err
	}
	if err := tx.q.DeleteGroupReport(ctx, r.Name); err != nil {
		return err
	}
	fields, err := groupFields(r)
	if err != nil {
		return err
	}
	before, err := pick(fields, slices.Collect(maps.Keys(fields)), "")
	if err != nil {
		return err
	}
	_, err = tx.Audit(ctx, audit.Event{Time: now, Actor: c.Actor, Action: audit.GroupRemoved, Target: audit.Target{Type: targetGroup, ID: r.Name},
		Before: before, Reason: c.Reason}.WithContext(ctx))
	return err
}

// seal returns a group's API key, as seal seals it for the place of the
// group whose ID is id, or empty if seal is nil. It refuses a key sealed
// with another master key than the database's other secrets.
func (tx *Tx) seal(ctx context.Context, id int64, seal Seal) (string, error) {
	if seal == nil {
		return "", nil
	}
	key, err := seal(GroupPlace(id))
	if err != nil {
		return "", err
	}
	keyID, err := secrets.KeyID(key)
	if err != nil {
		return "", fmt.Errorf("its API key must be sealed: %w", err)
	}
	return key, tx.checkMasterKey(ctx, keyID)
}

// bumpGroups bumps the groups' version, which a running serve watches.
func (tx *Tx) bumpGroups(ctx context.Context) error {
	_, err := tx.q.NextIDs(ctx, sqlite.NextIDsParams{N: 1, Name: seqGroupsVersion})
	return err
}

// groupFields returns r's fields as its events show them, by the names
// that the config file gives them, each as JSON: its spec's, its
// managed_by, and, if it stores one, its API key, as [redacted].
func groupFields(r sqlite.ServerGroup) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(r.SpecJson), &out); err != nil {
		return nil, err
	}
	var err error
	if out["managed_by"], err = json.Marshal(r.ManagedBy); err != nil {
		return nil, err
	}
	if r.ApiKey != "" {
		if out["primary.api_key"], err = json.Marshal(config.Redacted); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// pick returns those of fields that keys name, with check, if it isn't
// empty, for an event's before or after.
func pick(fields map[string]json.RawMessage, keys []string, check string) (json.RawMessage, error) {
	out := map[string]any{}
	for _, k := range keys {
		if v, ok := fields[k]; ok {
			out[k] = v
		}
	}
	if check != "" {
		out["check"] = check
	}
	return audit.Value(out)
}
