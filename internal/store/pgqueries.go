package store

import (
	"context"

	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store/postgres"
	"github.com/zeddD1abl0/netbox-powerdns-ai/internal/store/sqlite"
)

// pgQueries adapts sqlc's PostgreSQL package to the SQLite package's
// Querier, which the store calls on either database (ADR-0046). sqlc
// generates both packages from the same SQL, so each PostgreSQL struct has
// the same fields as its SQLite twin, and converts to it: the compiler
// checks every conversion, and fails if the two packages drift apart.
type pgQueries struct{ q *postgres.Queries }

var (
	_ sqlite.Querier = (*sqlite.Queries)(nil)
	_ sqlite.Querier = pgQueries{}
)

// one returns a function that converts a query's row with conv.
func one[F, T any](conv func(F) T) func(F, error) (T, error) {
	return func(r F, err error) (T, error) { return conv(r), err }
}

// all returns a function that converts each of a query's rows with conv.
func all[F, T any](conv func(F) T) func([]F, error) ([]T, error) {
	return func(rs []F, err error) ([]T, error) {
		if rs == nil {
			return nil, err
		}
		out := make([]T, len(rs))
		for i, r := range rs {
			out[i] = conv(r)
		}
		return out, err
	}
}

// The conversions of the PostgreSQL package's rows, by type.
var (
	pgAuditEvent              = func(r postgres.AuditEvent) sqlite.AuditEvent { return sqlite.AuditEvent(r) }
	pgGetAuditAnchorRow       = func(r postgres.GetAuditAnchorRow) sqlite.GetAuditAnchorRow { return sqlite.GetAuditAnchorRow(r) }
	pgGetDatabaseInfoRow      = func(r postgres.GetDatabaseInfoRow) sqlite.GetDatabaseInfoRow { return sqlite.GetDatabaseInfoRow(r) }
	pgServerGroup             = func(r postgres.ServerGroup) sqlite.ServerGroup { return sqlite.ServerGroup(r) }
	pgSetting                 = func(r postgres.Setting) sqlite.Setting { return sqlite.Setting(r) }
	pgGroupRefreshesRow       = func(r postgres.GroupRefreshesRow) sqlite.GroupRefreshesRow { return sqlite.GroupRefreshesRow(r) }
	pgDriftGroupReport        = func(r postgres.DriftGroupReport) sqlite.DriftGroupReport { return sqlite.DriftGroupReport(r) }
	pgLastAuditEventRow       = func(r postgres.LastAuditEventRow) sqlite.LastAuditEventRow { return sqlite.LastAuditEventRow(r) }
	pgLastAuditEventBeforeRow = func(r postgres.LastAuditEventBeforeRow) sqlite.LastAuditEventBeforeRow {
		return sqlite.LastAuditEventBeforeRow(r)
	}
	pgDriftRefreshGroup  = func(r postgres.DriftRefreshGroup) sqlite.DriftRefreshGroup { return sqlite.DriftRefreshGroup(r) }
	pgDriftRefresh       = func(r postgres.DriftRefresh) sqlite.DriftRefresh { return sqlite.DriftRefresh(r) }
	pgDriftZoneHistory   = func(r postgres.DriftZoneHistory) sqlite.DriftZoneHistory { return sqlite.DriftZoneHistory(r) }
	pgSealedGroupKeysRow = func(r postgres.SealedGroupKeysRow) sqlite.SealedGroupKeysRow { return sqlite.SealedGroupKeysRow(r) }
	pgSealedSettingsRow  = func(r postgres.SealedSettingsRow) sqlite.SealedSettingsRow { return sqlite.SealedSettingsRow(r) }
)

func (p pgQueries) AuditEventsAfter(ctx context.Context, arg sqlite.AuditEventsAfterParams) ([]sqlite.AuditEvent, error) {
	return all(pgAuditEvent)(p.q.AuditEventsAfter(ctx, postgres.AuditEventsAfterParams(arg)))
}

func (p pgQueries) CountRefreshGroupRows(ctx context.Context, groupName string) (int64, error) {
	return p.q.CountRefreshGroupRows(ctx, groupName)
}

func (p pgQueries) CountZoneRows(ctx context.Context, groupName string) (int64, error) {
	return p.q.CountZoneRows(ctx, groupName)
}

func (p pgQueries) DeleteAuditEventsThrough(ctx context.Context, seq int64) (int64, error) {
	return p.q.DeleteAuditEventsThrough(ctx, seq)
}

func (p pgQueries) DeleteGroup(ctx context.Context, id int64) error {
	return p.q.DeleteGroup(ctx, id)
}

func (p pgQueries) DeleteGroupReport(ctx context.Context, groupName string) error {
	return p.q.DeleteGroupReport(ctx, groupName)
}

func (p pgQueries) DeleteOrphanGroupReports(ctx context.Context) error {
	return p.q.DeleteOrphanGroupReports(ctx)
}

func (p pgQueries) DeleteRefreshGroupsBefore(ctx context.Context, cutoff string) error {
	return p.q.DeleteRefreshGroupsBefore(ctx, cutoff)
}

func (p pgQueries) DeleteRefreshesBefore(ctx context.Context, cutoff string) (int64, error) {
	return p.q.DeleteRefreshesBefore(ctx, cutoff)
}

func (p pgQueries) DeleteSetting(ctx context.Context, key string) (int64, error) {
	return p.q.DeleteSetting(ctx, key)
}

func (p pgQueries) DeleteZoneRowsBefore(ctx context.Context, cutoff string) (int64, error) {
	return p.q.DeleteZoneRowsBefore(ctx, cutoff)
}

func (p pgQueries) FirstAuditEvent(ctx context.Context) (int64, error) {
	return p.q.FirstAuditEvent(ctx)
}

func (p pgQueries) GetAuditAnchor(ctx context.Context) (sqlite.GetAuditAnchorRow, error) {
	return one(pgGetAuditAnchorRow)(p.q.GetAuditAnchor(ctx))
}

func (p pgQueries) GetDatabaseInfo(ctx context.Context) (sqlite.GetDatabaseInfoRow, error) {
	return one(pgGetDatabaseInfoRow)(p.q.GetDatabaseInfo(ctx))
}

func (p pgQueries) GetGroup(ctx context.Context, name string) (sqlite.ServerGroup, error) {
	return one(pgServerGroup)(p.q.GetGroup(ctx, name))
}

func (p pgQueries) GetSetting(ctx context.Context, key string) (sqlite.Setting, error) {
	return one(pgSetting)(p.q.GetSetting(ctx, key))
}

func (p pgQueries) GroupNames(ctx context.Context) ([]string, error) {
	return p.q.GroupNames(ctx)
}

func (p pgQueries) GroupRefreshes(ctx context.Context, arg sqlite.GroupRefreshesParams) ([]sqlite.GroupRefreshesRow, error) {
	return all(pgGroupRefreshesRow)(p.q.GroupRefreshes(ctx, postgres.GroupRefreshesParams(arg)))
}

func (p pgQueries) GroupReports(ctx context.Context) ([]sqlite.DriftGroupReport, error) {
	return all(pgDriftGroupReport)(p.q.GroupReports(ctx))
}

func (p pgQueries) InitDatabaseInfo(ctx context.Context, arg sqlite.InitDatabaseInfoParams) error {
	return p.q.InitDatabaseInfo(ctx, postgres.InitDatabaseInfoParams(arg))
}

func (p pgQueries) InsertAuditEvent(ctx context.Context, arg sqlite.InsertAuditEventParams) error {
	return p.q.InsertAuditEvent(ctx, postgres.InsertAuditEventParams(arg))
}

func (p pgQueries) InsertGroup(ctx context.Context, arg sqlite.InsertGroupParams) error {
	return p.q.InsertGroup(ctx, postgres.InsertGroupParams(arg))
}

func (p pgQueries) InsertRefresh(ctx context.Context, arg sqlite.InsertRefreshParams) error {
	return p.q.InsertRefresh(ctx, postgres.InsertRefreshParams(arg))
}

func (p pgQueries) InsertRefreshGroup(ctx context.Context, arg sqlite.InsertRefreshGroupParams) error {
	return p.q.InsertRefreshGroup(ctx, postgres.InsertRefreshGroupParams(arg))
}

func (p pgQueries) InsertZoneRow(ctx context.Context, arg sqlite.InsertZoneRowParams) error {
	return p.q.InsertZoneRow(ctx, postgres.InsertZoneRowParams(arg))
}

func (p pgQueries) LastAuditEvent(ctx context.Context) (sqlite.LastAuditEventRow, error) {
	return one(pgLastAuditEventRow)(p.q.LastAuditEvent(ctx))
}

func (p pgQueries) LastAuditEventBefore(ctx context.Context, cutoff string) (sqlite.LastAuditEventBeforeRow, error) {
	return one(pgLastAuditEventBeforeRow)(p.q.LastAuditEventBefore(ctx, cutoff))
}

func (p pgQueries) LastCompleteRefresh(ctx context.Context) (string, error) {
	return p.q.LastCompleteRefresh(ctx)
}

func (p pgQueries) LastGroupResults(ctx context.Context) ([]sqlite.DriftRefreshGroup, error) {
	return all(pgDriftRefreshGroup)(p.q.LastGroupResults(ctx))
}

func (p pgQueries) LastRefresh(ctx context.Context) (sqlite.DriftRefresh, error) {
	return one(pgDriftRefresh)(p.q.LastRefresh(ctx))
}

func (p pgQueries) LatestZoneRows(ctx context.Context, groupName string) ([]sqlite.DriftZoneHistory, error) {
	return all(pgDriftZoneHistory)(p.q.LatestZoneRows(ctx, groupName))
}

func (p pgQueries) ListAuditEvents(ctx context.Context, arg sqlite.ListAuditEventsParams) ([]sqlite.AuditEvent, error) {
	return all(pgAuditEvent)(p.q.ListAuditEvents(ctx, postgres.ListAuditEventsParams(arg)))
}

func (p pgQueries) ListGroups(ctx context.Context) ([]sqlite.ServerGroup, error) {
	return all(pgServerGroup)(p.q.ListGroups(ctx))
}

func (p pgQueries) ListSettings(ctx context.Context) ([]sqlite.Setting, error) {
	return all(pgSetting)(p.q.ListSettings(ctx))
}

func (p pgQueries) NextIDs(ctx context.Context, arg sqlite.NextIDsParams) (int64, error) {
	return p.q.NextIDs(ctx, postgres.NextIDsParams(arg))
}

func (p pgQueries) RenameGroup(ctx context.Context, arg sqlite.RenameGroupParams) error {
	return p.q.RenameGroup(ctx, postgres.RenameGroupParams(arg))
}

func (p pgQueries) RenameGroupReport(ctx context.Context, arg sqlite.RenameGroupReportParams) error {
	return p.q.RenameGroupReport(ctx, postgres.RenameGroupReportParams(arg))
}

func (p pgQueries) RenameRefreshGroupRows(ctx context.Context, arg sqlite.RenameRefreshGroupRowsParams) error {
	return p.q.RenameRefreshGroupRows(ctx, postgres.RenameRefreshGroupRowsParams(arg))
}

func (p pgQueries) RenameZoneRows(ctx context.Context, arg sqlite.RenameZoneRowsParams) error {
	return p.q.RenameZoneRows(ctx, postgres.RenameZoneRowsParams(arg))
}

func (p pgQueries) SealedGroupKeys(ctx context.Context) ([]sqlite.SealedGroupKeysRow, error) {
	return all(pgSealedGroupKeysRow)(p.q.SealedGroupKeys(ctx))
}

func (p pgQueries) SealedSettings(ctx context.Context) ([]sqlite.SealedSettingsRow, error) {
	return all(pgSealedSettingsRow)(p.q.SealedSettings(ctx))
}

func (p pgQueries) SequenceValue(ctx context.Context, name string) (int64, error) {
	return p.q.SequenceValue(ctx, name)
}

func (p pgQueries) SetAuditAnchor(ctx context.Context, arg sqlite.SetAuditAnchorParams) error {
	return p.q.SetAuditAnchor(ctx, postgres.SetAuditAnchorParams(arg))
}

func (p pgQueries) UpdateGroup(ctx context.Context, arg sqlite.UpdateGroupParams) error {
	return p.q.UpdateGroup(ctx, postgres.UpdateGroupParams(arg))
}

func (p pgQueries) UpdateGroupKey(ctx context.Context, arg sqlite.UpdateGroupKeyParams) error {
	return p.q.UpdateGroupKey(ctx, postgres.UpdateGroupKeyParams(arg))
}

func (p pgQueries) UpdateSealedSetting(ctx context.Context, arg sqlite.UpdateSealedSettingParams) error {
	return p.q.UpdateSealedSetting(ctx, postgres.UpdateSealedSettingParams(arg))
}

func (p pgQueries) UpsertGroupReport(ctx context.Context, arg sqlite.UpsertGroupReportParams) error {
	return p.q.UpsertGroupReport(ctx, postgres.UpsertGroupReportParams(arg))
}

func (p pgQueries) UpsertSetting(ctx context.Context, arg sqlite.UpsertSettingParams) error {
	return p.q.UpsertSetting(ctx, postgres.UpsertSettingParams(arg))
}

func (p pgQueries) ZoneHistory(ctx context.Context, arg sqlite.ZoneHistoryParams) ([]sqlite.DriftZoneHistory, error) {
	return all(pgDriftZoneHistory)(p.q.ZoneHistory(ctx, postgres.ZoneHistoryParams(arg)))
}
