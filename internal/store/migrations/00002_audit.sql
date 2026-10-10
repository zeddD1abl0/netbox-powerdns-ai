-- The audit trail (ADR-0039): every change of state, in order, each event
-- holding the SHA-384 of its canonical form, and the hash of the event
-- before it. The anchor is where the trail starts once a prune has deleted
-- its oldest events: the last deleted event's number and hash.
-- +goose Up
CREATE TABLE audit_events (
    seq BIGINT PRIMARY KEY,
    at TEXT NOT NULL,
    actor TEXT NOT NULL,
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    before_json TEXT,
    after_json TEXT,
    reason TEXT NOT NULL,
    outcome TEXT NOT NULL,
    trace_id TEXT NOT NULL,
    request_id TEXT NOT NULL,
    source_ip TEXT NOT NULL,
    prev_hash TEXT NOT NULL,
    hash TEXT NOT NULL
);
CREATE INDEX audit_events_at ON audit_events (at);
CREATE INDEX audit_events_action ON audit_events (action);
CREATE INDEX audit_events_target ON audit_events (target_type, target_id);

CREATE TABLE audit_anchor (
    singleton BIGINT PRIMARY KEY CHECK (singleton = 1),
    seq BIGINT NOT NULL,
    hash TEXT NOT NULL,
    pruned_at TEXT NOT NULL
);
