-- name: NextIDs :one
UPDATE sequences SET value = value + sqlc.arg(n) WHERE name = sqlc.arg(name) RETURNING value;

-- name: InsertRefresh :exec
INSERT INTO drift_refreshes (
    id, kind, cause, started_at, finished_at, outcome, error, zones_json, netbox_request_ids_json, trace_id
) VALUES (
    sqlc.arg(id), sqlc.arg(kind), sqlc.arg(cause), sqlc.arg(started_at), sqlc.arg(finished_at), sqlc.arg(outcome),
    sqlc.arg(error), sqlc.arg(zones_json), sqlc.arg(netbox_request_ids_json), sqlc.arg(trace_id)
);

-- name: InsertRefreshGroup :exec
INSERT INTO drift_refresh_groups (refresh_id, group_name, status, error, counts_json)
VALUES (sqlc.arg(refresh_id), sqlc.arg(group_name), sqlc.arg(status), sqlc.arg(error), sqlc.arg(counts_json));

-- name: LatestZoneRows :many
SELECT h.* FROM drift_zone_history h
WHERE h.group_name = sqlc.arg(group_name)
  AND h.id IN (SELECT MAX(l.id) FROM drift_zone_history l WHERE l.group_name = sqlc.arg(group_name) GROUP BY l.zone);

-- name: InsertZoneRow :exec
INSERT INTO drift_zone_history (
    id, group_name, zone, at, refresh_id, state, view_name, policy, netbox_serial, powerdns_serial, changes_json, digest
) VALUES (
    sqlc.arg(id), sqlc.arg(group_name), sqlc.arg(zone), sqlc.arg(at), sqlc.arg(refresh_id), sqlc.arg(state),
    sqlc.arg(view_name), sqlc.arg(policy), sqlc.arg(netbox_serial), sqlc.arg(powerdns_serial), sqlc.arg(changes_json),
    sqlc.arg(digest)
);

-- name: UpsertGroupReport :exec
INSERT INTO drift_group_reports (group_name, last_success, updated_at, report_json)
VALUES (sqlc.arg(group_name), sqlc.arg(last_success), sqlc.arg(updated_at), sqlc.arg(report_json))
ON CONFLICT (group_name) DO UPDATE SET
    last_success = excluded.last_success, updated_at = excluded.updated_at, report_json = excluded.report_json;

-- name: GroupReports :many
SELECT * FROM drift_group_reports ORDER BY group_name;

-- name: LastRefresh :one
SELECT * FROM drift_refreshes ORDER BY id DESC LIMIT 1;

-- name: LastCompleteRefresh :one
SELECT finished_at FROM drift_refreshes WHERE kind = 'full' AND outcome = 'complete' ORDER BY id DESC LIMIT 1;

-- name: LastGroupResults :many
SELECT * FROM drift_refresh_groups g
WHERE g.refresh_id = (SELECT MAX(l.refresh_id) FROM drift_refresh_groups l WHERE l.group_name = g.group_name)
ORDER BY g.group_name;

-- name: DeleteRefreshGroupsBefore :exec
DELETE FROM drift_refresh_groups
WHERE refresh_id IN (SELECT id FROM drift_refreshes WHERE finished_at < sqlc.arg(cutoff));

-- name: DeleteRefreshesBefore :execrows
DELETE FROM drift_refreshes WHERE finished_at < sqlc.arg(cutoff);

-- name: DeleteZoneRowsBefore :execrows
DELETE FROM drift_zone_history
WHERE drift_zone_history.at < sqlc.arg(cutoff)
  AND (drift_zone_history.state = 'removed'
    OR drift_zone_history.id NOT IN (SELECT MAX(l.id) FROM drift_zone_history l GROUP BY l.group_name, l.zone));

-- name: ZoneHistory :many
SELECT * FROM drift_zone_history
WHERE group_name = sqlc.arg(group_name) AND zone = sqlc.arg(zone) AND id < sqlc.arg(before_id)
ORDER BY id DESC
LIMIT sqlc.arg(max_rows);

-- name: GroupRefreshes :many
SELECT r.id, r.kind, r.cause, r.started_at, r.finished_at, r.outcome, r.error, r.zones_json,
    r.netbox_request_ids_json, r.trace_id, g.status AS group_status, g.error AS group_error, g.counts_json
FROM drift_refreshes r
LEFT JOIN drift_refresh_groups g ON g.refresh_id = r.id AND g.group_name = sqlc.arg(group_name)
WHERE r.id < sqlc.arg(before_id)
ORDER BY r.id DESC
LIMIT sqlc.arg(max_rows);
