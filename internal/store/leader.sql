-- name: GetLeader :one
SELECT replica, instance_id, since, last_seen, next_refresh FROM leader WHERE singleton = 1;

-- name: SetLeader :exec
INSERT INTO leader (singleton, replica, instance_id, since, last_seen, next_refresh)
VALUES (1, sqlc.arg(replica), sqlc.arg(instance_id), sqlc.arg(since), sqlc.arg(last_seen), sqlc.arg(next_refresh))
ON CONFLICT (singleton) DO UPDATE SET replica = excluded.replica, instance_id = excluded.instance_id,
    since = excluded.since, last_seen = excluded.last_seen, next_refresh = excluded.next_refresh;

-- name: LeaderHeartbeat :execrows
UPDATE leader SET last_seen = sqlc.arg(last_seen), next_refresh = sqlc.arg(next_refresh)
WHERE singleton = 1 AND instance_id = sqlc.arg(instance_id);
