package store

import (
	"cmp"
	"context"
	"crypto/sha512"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/audit"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/drift"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store/sqlite"
)

// The kinds of refresh.
const (
	// RefreshFull compares every zone.
	RefreshFull = "full"
	// RefreshZones compares the zones that NetBox's webhooks named
	// (ADR-0036).
	RefreshZones = "zones"
)

// The causes of a refresh.
const (
	CauseSchedule = "schedule"
	CauseWebhook  = "webhook"
)

// StateRemoved is a zone's state in its last history row, once it has left
// its group's report: it's in neither the group's NetBox views nor its
// primary.
const StateRemoved = "removed"

// The sequences that IDs come from.
const (
	seqRefreshes = "drift_refreshes"
	seqZoneRows  = "drift_zone_history"
)

// A RefreshRecord is a refresh, as the drift history keeps it (ADR-0042).
type RefreshRecord struct {
	// Kind is RefreshFull or RefreshZones, and Cause CauseSchedule or
	// CauseWebhook.
	Kind, Cause       string
	Started, Finished time.Time
	// Outcome is complete, incomplete or failed, and Error why it failed.
	Outcome, Error string
	// Zones are those that a zone refresh compared, as view/name.
	Zones []string
	// NetBoxRequestIDs are those of the NetBox changes whose webhooks
	// asked for the refresh.
	NetBoxRequestIDs []string
	TraceID          string
	// Groups are the result of each group that the refresh tried.
	Groups []GroupResult
	// Reports are the reports kept for the groups that were read: for a
	// zone refresh, merged into the last.
	Reports []KeptReport
}

// A GroupResult is how a refresh went for a group.
type GroupResult struct {
	Group, Status, Error string
	Counts               drift.Counts
}

// A KeptReport is a group's report, as the service keeps it.
type KeptReport struct {
	Report drift.GroupReport
	// LastSuccess is when the group was last compared in full.
	LastSuccess time.Time
}

// RecordRefresh writes r to the drift history, in one transaction: the
// refresh and each group's result; a row for each zone in r's reports whose
// state, or the digest of its changes, differs from its last row, and a
// removed row for each zone that has left its group's report; and each
// group's report, replacing its last. Each zone's change of state is also a
// zone.drift_changed event (ADR-0008, ADR-0039), with ctx's trace and request
// IDs, as is a new zone's, if it drifted. Only the groups that the database
// stores are written. It returns how many zone rows it wrote, and the
// events.
func (s *Store) RecordRefresh(ctx context.Context, r RefreshRecord) (int, []audit.Event, error) {
	rows := 0
	events, err := s.InTx(ctx, func(tx *Tx) error {
		names, err := tx.q.GroupNames(ctx)
		if err != nil {
			return err
		}
		// A refresh that a group's removal or rename overtook has results
		// under a name that the history no longer keeps (ADR-0045).
		stored := func(group string) bool { return slices.Contains(names, group) }
		id, err := tx.q.NextIDs(ctx, sqlite.NextIDsParams{N: 1, Name: seqRefreshes})
		if err != nil {
			return err
		}
		zones, err := jsonText(nonNil(r.Zones))
		if err != nil {
			return err
		}
		requests, err := jsonText(nonNil(r.NetBoxRequestIDs))
		if err != nil {
			return err
		}
		finished := r.Finished.UTC().Format(TimeLayout)
		if err := tx.q.InsertRefresh(ctx, sqlite.InsertRefreshParams{
			ID: id, Kind: r.Kind, Cause: r.Cause, StartedAt: r.Started.UTC().Format(TimeLayout), FinishedAt: finished,
			Outcome: r.Outcome, Error: r.Error, ZonesJson: zones, NetboxRequestIdsJson: requests, TraceID: r.TraceID,
		}); err != nil {
			return err
		}
		for _, g := range r.Groups {
			if !stored(g.Group) {
				continue
			}
			counts, err := jsonText(g.Counts)
			if err != nil {
				return err
			}
			if err := tx.q.InsertRefreshGroup(ctx, sqlite.InsertRefreshGroupParams{
				RefreshID: id, GroupName: g.Group, Status: g.Status, Error: g.Error, CountsJson: counts,
			}); err != nil {
				return err
			}
		}
		for _, k := range r.Reports {
			if !stored(k.Report.Group) {
				continue
			}
			n, err := tx.recordZones(ctx, id, finished, k.Report)
			if err != nil {
				return err
			}
			rows += n
			report, err := encodeReport(k.Report)
			if err != nil {
				return err
			}
			if err := tx.q.UpsertGroupReport(ctx, sqlite.UpsertGroupReportParams{
				GroupName: k.Report.Group, LastSuccess: k.LastSuccess.UTC().Format(TimeLayout), UpdatedAt: finished,
				ReportJson: report,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, nil, fmt.Errorf("writing the refresh to the drift history: %w", err)
	}
	return rows, events, nil
}

// A zoneRow is a zone's history row, to be written.
type zoneRow struct {
	zone, state, view, policy string
	netboxSerial, pdnsSerial  int64
	changes, digest           string
	// prev is the zone's state in its last row, or empty if it has none.
	prev string
	// changeCount is how many of its RRsets differ.
	changeCount int
}

// recordZones writes the rows of g's zones that differ from their last
// rows, and the events of those whose state changed, in refresh id,
// finished at at, and returns how many rows it wrote.
func (t *Tx) recordZones(ctx context.Context, id int64, at string, g drift.GroupReport) (int, error) {
	latest, err := t.q.LatestZoneRows(ctx, g.Group)
	if err != nil {
		return 0, err
	}
	last := make(map[string]sqlite.DriftZoneHistory, len(latest))
	for _, r := range latest {
		last[r.Zone] = r
	}
	var rows []zoneRow
	seen := map[string]bool{}
	for _, z := range g.Zones {
		seen[z.Zone] = true
		changes, digest, err := digestOf(z.Changes)
		if err != nil {
			return 0, err
		}
		prev, had := last[z.Zone]
		if had && prev.State == z.State && prev.Digest == digest {
			continue
		}
		row := zoneRow{zone: z.Zone, state: z.State, view: z.View, policy: z.Policy, netboxSerial: int64(z.NetBoxSerial),
			pdnsSerial: int64(z.PowerDNSSerial), changes: changes, digest: digest, changeCount: len(z.Changes)}
		if had && prev.State != StateRemoved {
			row.prev = prev.State
		}
		rows = append(rows, row)
	}
	for zone, prev := range last {
		if seen[zone] || prev.State == StateRemoved {
			continue
		}
		changes, digest, err := digestOf(nil)
		if err != nil {
			return 0, err
		}
		rows = append(rows, zoneRow{zone: zone, state: StateRemoved, view: prev.ViewName, policy: prev.Policy,
			changes: changes, digest: digest, prev: prev.State})
	}
	if len(rows) == 0 {
		return 0, nil
	}
	slices.SortFunc(rows, func(a, b zoneRow) int { return cmp.Compare(a.zone, b.zone) })
	end, err := t.q.NextIDs(ctx, sqlite.NextIDsParams{N: int64(len(rows)), Name: seqZoneRows})
	if err != nil {
		return 0, err
	}
	when, err := time.Parse(TimeLayout, at)
	if err != nil {
		return 0, err
	}
	for i, r := range rows {
		if err := t.q.InsertZoneRow(ctx, sqlite.InsertZoneRowParams{
			ID: end - int64(len(rows)) + 1 + int64(i), GroupName: g.Group, Zone: r.zone, At: at, RefreshID: id,
			State: r.state, ViewName: r.view, Policy: r.policy, NetboxSerial: r.netboxSerial, PowerdnsSerial: r.pdnsSerial,
			ChangesJson: r.changes, Digest: r.digest,
		}); err != nil {
			return 0, err
		}
		if !stateChanged(r) {
			continue
		}
		if err := t.zoneEvent(ctx, when, g.Group, r); err != nil {
			return 0, err
		}
	}
	return len(rows), nil
}

// stateChanged reports whether r records a change of state worth an audit
// event: from the zone's last state, or, for a zone with none, to a drifted
// state, so that the first refresh of a database doesn't record every zone
// in sync.
func stateChanged(r zoneRow) bool {
	if r.prev == "" {
		return drift.IsDrifted(r.state)
	}
	return r.prev != r.state
}

// zoneEvent records r's change of state as a zone.drift_changed event.
func (t *Tx) zoneEvent(ctx context.Context, at time.Time, group string, r zoneRow) error {
	var before json.RawMessage
	if r.prev != "" {
		var err error
		if before, err = audit.Value(map[string]string{"state": r.prev}); err != nil {
			return err
		}
	}
	after, err := audit.Value(map[string]any{"state": r.state, "changes": r.changeCount, "policy": r.policy})
	if err != nil {
		return err
	}
	_, err = t.Audit(ctx, audit.Event{
		Time: at, Actor: audit.System(), Action: audit.ZoneDriftChanged,
		Target: audit.Target{Type: "zone", ID: group + "/" + r.zone}, Before: before, After: after,
	}.WithContext(ctx))
	return err
}

// digestOf returns changes as JSON, and the hex SHA-384 of that JSON, with
// the changes sorted, so that the same changes always have the same digest.
func digestOf(changes []drift.Change) (string, string, error) {
	sorted := slices.Clone(changes)
	slices.SortFunc(sorted, func(a, b drift.Change) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Type, b.Type), cmp.Compare(a.Kind, b.Kind))
	})
	text, err := jsonText(nonNil(sorted))
	if err != nil {
		return "", "", err
	}
	sum := sha512.Sum384([]byte(text))
	return text, hex.EncodeToString(sum[:]), nil
}

// A storedReport is a group's report as the history keeps it: a warning's
// JSON is only its text, so the warnings, with their zones, are kept beside
// it.
type storedReport struct {
	Report   drift.GroupReport `json:"report"`
	Warnings []storedWarning   `json:"warnings"`
}

type storedWarning struct {
	Zone string `json:"zone"`
	Text string `json:"text"`
}

func encodeReport(g drift.GroupReport) (string, error) {
	sr := storedReport{Report: g, Warnings: make([]storedWarning, len(g.Warnings))}
	for i, w := range g.Warnings {
		sr.Warnings[i] = storedWarning{Zone: w.Zone, Text: w.Text}
	}
	return jsonText(sr)
}

func decodeReport(s string) (drift.GroupReport, error) {
	var sr storedReport
	if err := json.Unmarshal([]byte(s), &sr); err != nil {
		return drift.GroupReport{}, err
	}
	g := sr.Report
	g.Warnings = make([]drift.Warning, len(sr.Warnings))
	for i, w := range sr.Warnings {
		g.Warnings[i] = drift.Warning{Zone: w.Zone, Text: w.Text}
	}
	return g, nil
}

// A RefreshSummary is a refresh, as Restore reads it back.
type RefreshSummary struct {
	Kind, Cause       string
	Started, Finished time.Time
	Outcome, Error    string
}

// Restored is the drift state that the history kept, for serve to start
// from.
type Restored struct {
	// Last is the last refresh, or nil if there's none.
	Last *RefreshSummary
	// LastComplete is when the last complete full refresh finished, or
	// zero.
	LastComplete time.Time
	// Groups holds each group's state, by name.
	Groups map[string]RestoredGroup
}

// A RestoredGroup is a group's last-known state.
type RestoredGroup struct {
	// Report is its last report, if it has one.
	Report *drift.GroupReport
	// LastSuccess is when it was last compared in full, if it ever was.
	LastSuccess time.Time
	// Status and Error are its last refresh's result, if it has one.
	Status, Error string
}

// RestoreDrift reads back the last refresh, and each group's last report and
// result, in one read transaction.
func (s *Store) RestoreDrift(ctx context.Context) (Restored, error) {
	out := Restored{Groups: map[string]RestoredGroup{}}
	sqlTx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, fmt.Errorf("restoring the drift report: %w", err)
	}
	defer func() { _ = sqlTx.Rollback() }()
	q := s.q.WithTx(sqlTx)
	fail := func(err error) (Restored, error) { return out, fmt.Errorf("restoring the drift report: %w", err) }

	switch r, err := q.LastRefresh(ctx); {
	case err == nil:
		sum := RefreshSummary{Kind: r.Kind, Cause: r.Cause, Outcome: r.Outcome, Error: r.Error}
		if sum.Started, err = time.Parse(TimeLayout, r.StartedAt); err != nil {
			return fail(err)
		}
		if sum.Finished, err = time.Parse(TimeLayout, r.FinishedAt); err != nil {
			return fail(err)
		}
		out.Last = &sum
	case !errors.Is(err, sql.ErrNoRows):
		return fail(err)
	}
	switch at, err := q.LastCompleteRefresh(ctx); {
	case err == nil:
		if out.LastComplete, err = time.Parse(TimeLayout, at); err != nil {
			return fail(err)
		}
	case !errors.Is(err, sql.ErrNoRows):
		return fail(err)
	}
	reports, err := q.GroupReports(ctx)
	if err != nil {
		return fail(err)
	}
	for _, r := range reports {
		g, err := decodeReport(r.ReportJson)
		if err != nil {
			return fail(fmt.Errorf("group %s's report: %w", r.GroupName, err))
		}
		last, err := time.Parse(TimeLayout, r.LastSuccess)
		if err != nil {
			return fail(err)
		}
		out.Groups[r.GroupName] = RestoredGroup{Report: &g, LastSuccess: last}
	}
	results, err := q.LastGroupResults(ctx)
	if err != nil {
		return fail(err)
	}
	for _, r := range results {
		rg := out.Groups[r.GroupName]
		rg.Status, rg.Error = r.Status, r.Error
		out.Groups[r.GroupName] = rg
	}
	return out, nil
}

// PruneDriftHistory deletes the refreshes that finished before cutoff, with
// their groups' results, and the zone rows written before it, but for each
// zone's last row, unless that row says the zone was removed, or its group
// is. It also deletes the last reports of groups that the database no
// longer stores. It returns how many refreshes and zone rows it deleted.
func (s *Store) PruneDriftHistory(ctx context.Context, cutoff time.Time) (refreshes, rows int64, err error) {
	at := cutoff.UTC().Format(TimeLayout)
	_, err = s.InTx(ctx, func(tx *Tx) error {
		if err := tx.q.DeleteRefreshGroupsBefore(ctx, at); err != nil {
			return err
		}
		var err error
		if refreshes, err = tx.q.DeleteRefreshesBefore(ctx, at); err != nil {
			return err
		}
		if rows, err = tx.q.DeleteZoneRowsBefore(ctx, at); err != nil {
			return err
		}
		return tx.q.DeleteOrphanGroupReports(ctx)
	})
	if err != nil {
		return 0, 0, fmt.Errorf("pruning the drift history: %w", err)
	}
	return refreshes, rows, nil
}

// jsonText returns v as JSON text.
func jsonText(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// nonNil returns s, or an empty slice if it's nil, so that it's written as
// [] rather than null.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// A ZoneHistoryRow is one row of a zone's drift history.
type ZoneHistoryRow struct {
	ID int64
	// At is when the refresh that wrote it finished.
	At        time.Time
	RefreshID int64
	// State is the zone's drift state, or StateRemoved.
	State, View, Policy          string
	NetBoxSerial, PowerDNSSerial uint32
	// Changes are its RRsets that differed, in canonical order.
	Changes []drift.Change
}

// ZoneHistory returns up to limit of a zone's rows in a group, newest
// first, from the row before the row with ID before, or from the newest if
// before is 0.
func (s *Store) ZoneHistory(ctx context.Context, group, zone string, before int64, limit int) ([]ZoneHistoryRow, error) {
	rows, err := s.q.ZoneHistory(ctx, sqlite.ZoneHistoryParams{GroupName: group, Zone: zone, BeforeID: beforeID(before), MaxRows: int64(limit)})
	if err != nil {
		return nil, fmt.Errorf("reading a zone's drift history: %w", err)
	}
	out := make([]ZoneHistoryRow, len(rows))
	for i, r := range rows {
		at, err := time.Parse(TimeLayout, r.At)
		if err != nil {
			return nil, fmt.Errorf("reading a zone's drift history: %w", err)
		}
		row := ZoneHistoryRow{ID: r.ID, At: at, RefreshID: r.RefreshID, State: r.State, View: r.ViewName, Policy: r.Policy,
			NetBoxSerial: uint32(r.NetboxSerial), PowerDNSSerial: uint32(r.PowerdnsSerial)} //nolint:gosec // Serials were uint32s.
		if err := json.Unmarshal([]byte(r.ChangesJson), &row.Changes); err != nil {
			return nil, fmt.Errorf("reading a zone's drift history: row %d's changes: %w", r.ID, err)
		}
		out[i] = row
	}
	return out, nil
}

// A RefreshRow is a refresh, with a group's result in it.
type RefreshRow struct {
	ID int64
	RefreshSummary
	// Zones are those that a zone refresh compared, as view/name, or nil
	// for a full refresh.
	Zones            []string
	NetBoxRequestIDs []string
	TraceID          string
	// Group is the group's result, or nil if the refresh didn't try the
	// group, as one that couldn't read NetBox doesn't.
	Group *GroupResult
}

// Refreshes returns up to limit refreshes, newest first, from the refresh
// before the one with ID before, or from the newest if before is 0, each
// with group's result in it.
func (s *Store) Refreshes(ctx context.Context, group string, before int64, limit int) ([]RefreshRow, error) {
	rows, err := s.q.GroupRefreshes(ctx, sqlite.GroupRefreshesParams{GroupName: group, BeforeID: beforeID(before), MaxRows: int64(limit)})
	if err != nil {
		return nil, fmt.Errorf("reading the refreshes: %w", err)
	}
	out := make([]RefreshRow, len(rows))
	for i, r := range rows {
		fail := func(err error) ([]RefreshRow, error) {
			return nil, fmt.Errorf("reading the refreshes: refresh %d: %w", r.ID, err)
		}
		row := RefreshRow{ID: r.ID, RefreshSummary: RefreshSummary{Kind: r.Kind, Cause: r.Cause, Outcome: r.Outcome, Error: r.Error},
			TraceID: r.TraceID}
		if row.Started, err = time.Parse(TimeLayout, r.StartedAt); err != nil {
			return fail(err)
		}
		if row.Finished, err = time.Parse(TimeLayout, r.FinishedAt); err != nil {
			return fail(err)
		}
		if err := json.Unmarshal([]byte(r.NetboxRequestIdsJson), &row.NetBoxRequestIDs); err != nil {
			return fail(err)
		}
		if r.Kind == RefreshZones {
			if err := json.Unmarshal([]byte(r.ZonesJson), &row.Zones); err != nil {
				return fail(err)
			}
		}
		if r.GroupStatus.Valid {
			g := GroupResult{Group: group, Status: r.GroupStatus.String, Error: r.GroupError.String}
			if err := json.Unmarshal([]byte(r.CountsJson.String), &g.Counts); err != nil {
				return fail(err)
			}
			row.Group = &g
		}
		out[i] = row
	}
	return out, nil
}

// beforeID returns the ID that a list starts before: before, or, for 0,
// past every ID.
func beforeID(before int64) int64 {
	if before <= 0 {
		return math.MaxInt64
	}
	return before
}
