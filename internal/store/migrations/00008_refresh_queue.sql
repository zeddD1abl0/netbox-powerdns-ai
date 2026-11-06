-- NetBox's webhooks, which every replica takes (ADR-0048, ADR-0036). Each
-- webhook that asks for a refresh is a row of refresh_queue: the zones it
-- names, in views that a server group serves, or a full refresh, for its
-- reason; NetBox's request and user; and its span's IDs, which the refresh
-- links to. The leader gathers the queue, and a refresh that covered it
-- deletes the rows up to the last it gathered, in the transaction that
-- records it, so that a refresh that isn't recorded leaves them queued.
-- webhook_state holds the rest that /status shows: covered_through, the
-- last row that a recorded refresh covered; the last webhook's event,
-- whether it asked for a refresh or not, with an empty last_event_at
-- before the first; and the last refresh that webhooks asked for, or that
-- covered the zones they named, as JSON, or empty before the first. IDs
-- come from the refresh_queue sequence, which each webhook, and each
-- refresh that covers the queue, moves: it's the webhooks' version, which
-- the replicas follow.
-- +goose Up
CREATE TABLE refresh_queue (
    id BIGINT PRIMARY KEY,
    received_at TEXT NOT NULL,
    zones_json TEXT NOT NULL,
    full_refresh BOOLEAN NOT NULL,
    reason TEXT NOT NULL,
    request_id TEXT NOT NULL,
    request_user TEXT NOT NULL,
    trace_id TEXT NOT NULL,
    span_id TEXT NOT NULL
);

CREATE TABLE webhook_state (
    singleton BIGINT PRIMARY KEY CHECK (singleton = 1),
    covered_through BIGINT NOT NULL,
    last_event_at TEXT NOT NULL,
    last_event TEXT NOT NULL,
    last_object_type TEXT NOT NULL,
    last_request_id TEXT NOT NULL,
    last_request_user TEXT NOT NULL,
    last_refresh_json TEXT NOT NULL
);
INSERT INTO webhook_state (
    singleton, covered_through, last_event_at, last_event, last_object_type, last_request_id, last_request_user,
    last_refresh_json
) VALUES (1, 0, '', '', '', '', '', '');
INSERT INTO sequences (name, value) VALUES ('refresh_queue', 0);
