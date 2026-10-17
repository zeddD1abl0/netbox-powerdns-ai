-- name: LastAuditEvent :one
SELECT seq, hash FROM audit_events ORDER BY seq DESC LIMIT 1;

-- name: FirstAuditEvent :one
SELECT seq FROM audit_events ORDER BY seq LIMIT 1;

-- name: GetAuditAnchor :one
SELECT seq, hash FROM audit_anchor WHERE singleton = 1;

-- name: InsertAuditEvent :exec
INSERT INTO audit_events (
    seq, at, actor, action, target_type, target_id, before_json, after_json, reason, outcome,
    trace_id, request_id, source_ip, prev_hash, hash
) VALUES (
    sqlc.arg(seq), sqlc.arg(at), sqlc.arg(actor), sqlc.arg(action), sqlc.arg(target_type), sqlc.arg(target_id),
    sqlc.narg(before_json), sqlc.narg(after_json), sqlc.arg(reason), sqlc.arg(outcome),
    sqlc.arg(trace_id), sqlc.arg(request_id), sqlc.arg(source_ip), sqlc.arg(prev_hash), sqlc.arg(hash)
);

-- name: ListAuditEvents :many
SELECT * FROM audit_events
WHERE (sqlc.narg(since) IS NULL OR at >= sqlc.narg(since))
  AND (sqlc.narg(until) IS NULL OR at < sqlc.narg(until))
  AND (sqlc.narg(action) IS NULL OR action = sqlc.narg(action))
  AND (sqlc.narg(actor) IS NULL OR actor = sqlc.narg(actor))
  AND (sqlc.narg(target_type) IS NULL OR target_type = sqlc.narg(target_type))
  AND (sqlc.narg(target_id) IS NULL OR target_id = sqlc.narg(target_id))
ORDER BY seq DESC
LIMIT sqlc.arg(max_events);

-- name: AuditEventsAfter :many
SELECT * FROM audit_events WHERE seq > sqlc.arg(after) ORDER BY seq LIMIT sqlc.arg(max_events);

-- name: LastAuditEventBefore :one
-- The last event of those before the first one at or after cutoff, by seq:
-- a prune deletes the trail from its start, and an event can follow one
-- with a later time, as a refresh's do, which carry its end's time.
SELECT e.seq, e.hash FROM audit_events e
WHERE e.at < sqlc.arg(cutoff)
  AND NOT EXISTS (SELECT 1 FROM audit_events n WHERE n.seq < e.seq AND n.at >= sqlc.arg(cutoff))
ORDER BY e.seq DESC
LIMIT 1;

-- name: DeleteAuditEventsThrough :execrows
DELETE FROM audit_events WHERE seq <= sqlc.arg(seq);

-- name: SetAuditAnchor :exec
INSERT INTO audit_anchor (singleton, seq, hash, pruned_at)
VALUES (1, sqlc.arg(seq), sqlc.arg(hash), sqlc.arg(pruned_at))
ON CONFLICT (singleton) DO UPDATE SET seq = excluded.seq, hash = excluded.hash, pruned_at = excluded.pruned_at;
