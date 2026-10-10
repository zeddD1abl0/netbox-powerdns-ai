-- name: InitDatabaseInfo :exec
INSERT INTO database_info (singleton, id, created_at, created_by)
VALUES (1, sqlc.arg(id), sqlc.arg(created_at), sqlc.arg(created_by))
ON CONFLICT (singleton) DO NOTHING;

-- name: GetDatabaseInfo :one
SELECT id, created_at, created_by FROM database_info WHERE singleton = 1;
