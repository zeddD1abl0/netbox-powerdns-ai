-- name: NetBoxZoneIndex :many
SELECT view_name, name, refresh_id, digest FROM netbox_zones;

-- name: NetBoxZonesAfter :many
SELECT view_name, name, zone_json FROM netbox_zones WHERE refresh_id > sqlc.arg(after);

-- name: UpsertNetBoxZone :exec
INSERT INTO netbox_zones (view_name, name, refresh_id, digest, zone_json)
VALUES (sqlc.arg(view_name), sqlc.arg(name), sqlc.arg(refresh_id), sqlc.arg(digest), sqlc.arg(zone_json))
ON CONFLICT (view_name, name) DO UPDATE SET
    refresh_id = excluded.refresh_id, digest = excluded.digest, zone_json = excluded.zone_json;

-- name: DeleteNetBoxZone :exec
DELETE FROM netbox_zones WHERE view_name = sqlc.arg(view_name) AND name = sqlc.arg(name);

-- name: GetNetBoxState :one
SELECT status, error, zones_as_of FROM netbox_state WHERE singleton = 1;

-- name: SetNetBoxStatus :exec
UPDATE netbox_state SET status = sqlc.arg(status), error = sqlc.arg(error) WHERE singleton = 1;

-- name: SetNetBoxZonesAsOf :exec
UPDATE netbox_state SET zones_as_of = sqlc.arg(zones_as_of) WHERE singleton = 1;
