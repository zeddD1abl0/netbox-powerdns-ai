-- name: ListSettings :many
SELECT * FROM settings ORDER BY key;

-- name: GetSetting :one
SELECT * FROM settings WHERE key = sqlc.arg(key);

-- name: UpsertSetting :exec
INSERT INTO settings (key, value, updated_at, updated_by)
VALUES (sqlc.arg(key), sqlc.arg(value), sqlc.arg(updated_at), sqlc.arg(updated_by))
ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by;

-- name: DeleteSetting :execrows
DELETE FROM settings WHERE key = sqlc.arg(key);

-- name: SequenceValue :one
SELECT value FROM sequences WHERE name = sqlc.arg(name);
