-- name: InsertQueuedRefresh :exec
INSERT INTO refresh_queue (
    id, received_at, zones_json, full_refresh, reason, request_id, request_user, trace_id, span_id
) VALUES (
    sqlc.arg(id), sqlc.arg(received_at), sqlc.arg(zones_json), sqlc.arg(full_refresh), sqlc.arg(reason),
    sqlc.arg(request_id), sqlc.arg(request_user), sqlc.arg(trace_id), sqlc.arg(span_id)
);

-- name: QueuedRefreshesAfter :many
SELECT id, received_at, zones_json, full_refresh, reason, request_id, request_user, trace_id, span_id
FROM refresh_queue WHERE id > sqlc.arg(after) ORDER BY id;

-- name: DeleteQueuedRefreshesThrough :execrows
DELETE FROM refresh_queue WHERE id <= sqlc.arg(through);

-- name: GetWebhookState :one
SELECT covered_through, last_event_at, last_event, last_object_type, last_request_id, last_request_user, last_refresh_json
FROM webhook_state WHERE singleton = 1;

-- name: SetLastWebhookEvent :exec
UPDATE webhook_state SET last_event_at = sqlc.arg(last_event_at), last_event = sqlc.arg(last_event),
    last_object_type = sqlc.arg(last_object_type), last_request_id = sqlc.arg(last_request_id),
    last_request_user = sqlc.arg(last_request_user)
WHERE singleton = 1;

-- name: SetQueueCovered :exec
UPDATE webhook_state SET covered_through = sqlc.arg(covered_through), last_refresh_json = sqlc.arg(last_refresh_json)
WHERE singleton = 1;
