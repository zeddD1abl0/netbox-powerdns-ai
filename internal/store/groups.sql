-- name: ListGroups :many
SELECT * FROM server_groups ORDER BY name;

-- name: GetGroup :one
SELECT * FROM server_groups WHERE name = sqlc.arg(name);

-- name: InsertGroup :exec
INSERT INTO server_groups (id, name, managed_by, spec_json, api_key, created_at, created_by, updated_at, updated_by)
VALUES (
    sqlc.arg(id), sqlc.arg(name), sqlc.arg(managed_by), sqlc.arg(spec_json), sqlc.arg(api_key), sqlc.arg(created_at),
    sqlc.arg(created_by), sqlc.arg(updated_at), sqlc.arg(updated_by)
);

-- name: UpdateGroup :exec
UPDATE server_groups
SET spec_json = sqlc.arg(spec_json), api_key = sqlc.arg(api_key), updated_at = sqlc.arg(updated_at), updated_by = sqlc.arg(updated_by)
WHERE id = sqlc.arg(id);

-- name: RenameGroup :exec
UPDATE server_groups SET name = sqlc.arg(new_name), updated_at = sqlc.arg(updated_at), updated_by = sqlc.arg(updated_by)
WHERE id = sqlc.arg(id);

-- name: DeleteGroup :exec
DELETE FROM server_groups WHERE id = sqlc.arg(id);

-- name: SealedGroupKeys :many
SELECT id, name, api_key FROM server_groups WHERE api_key <> '' ORDER BY id;

-- name: UpdateGroupKey :exec
UPDATE server_groups SET api_key = sqlc.arg(api_key) WHERE id = sqlc.arg(id);

-- name: GroupNames :many
SELECT name FROM server_groups;

-- name: DeleteGroupReport :exec
DELETE FROM drift_group_reports WHERE group_name = sqlc.arg(group_name);

-- name: CountRefreshGroupRows :one
SELECT count(*) FROM drift_refresh_groups WHERE group_name = sqlc.arg(group_name);

-- name: CountZoneRows :one
SELECT count(*) FROM drift_zone_history WHERE group_name = sqlc.arg(group_name);

-- name: RenameRefreshGroupRows :exec
UPDATE drift_refresh_groups SET group_name = sqlc.arg(new_name) WHERE group_name = sqlc.arg(old_name);

-- name: RenameZoneRows :exec
UPDATE drift_zone_history SET group_name = sqlc.arg(new_name) WHERE group_name = sqlc.arg(old_name);

-- name: RenameGroupReport :exec
UPDATE drift_group_reports SET group_name = sqlc.arg(new_name) WHERE group_name = sqlc.arg(old_name);

-- name: DeleteOrphanGroupReports :exec
DELETE FROM drift_group_reports WHERE group_name NOT IN (SELECT name FROM server_groups);
