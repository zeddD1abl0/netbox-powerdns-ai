-- Drift history (ADR-0042): each refresh, and each group's result in it; a
-- zone's row whenever its state, or the digest of its RRset changes,
-- differs from its last row; and each group's last report, kept whole and
-- replaced at each refresh, to restore at start. IDs come from sequences,
-- so that a prune never lets one be used again.
-- +goose Up
CREATE TABLE sequences (
    name TEXT PRIMARY KEY,
    value BIGINT NOT NULL
);
INSERT INTO sequences (name, value) VALUES ('drift_refreshes', 0), ('drift_zone_history', 0);

CREATE TABLE drift_refreshes (
    id BIGINT PRIMARY KEY,
    kind TEXT NOT NULL,
    cause TEXT NOT NULL,
    started_at TEXT NOT NULL,
    finished_at TEXT NOT NULL,
    outcome TEXT NOT NULL,
    error TEXT NOT NULL,
    zones_json TEXT NOT NULL,
    netbox_request_ids_json TEXT NOT NULL,
    trace_id TEXT NOT NULL
);
CREATE INDEX drift_refreshes_finished ON drift_refreshes (finished_at);

CREATE TABLE drift_refresh_groups (
    refresh_id BIGINT NOT NULL,
    group_name TEXT NOT NULL,
    status TEXT NOT NULL,
    error TEXT NOT NULL,
    counts_json TEXT NOT NULL,
    PRIMARY KEY (refresh_id, group_name)
);
CREATE INDEX drift_refresh_groups_group ON drift_refresh_groups (group_name, refresh_id);

CREATE TABLE drift_zone_history (
    id BIGINT PRIMARY KEY,
    group_name TEXT NOT NULL,
    zone TEXT NOT NULL,
    at TEXT NOT NULL,
    refresh_id BIGINT NOT NULL,
    state TEXT NOT NULL,
    view_name TEXT NOT NULL,
    policy TEXT NOT NULL,
    netbox_serial BIGINT NOT NULL,
    powerdns_serial BIGINT NOT NULL,
    changes_json TEXT NOT NULL,
    digest TEXT NOT NULL
);
CREATE INDEX drift_zone_history_zone ON drift_zone_history (group_name, zone, id);
CREATE INDEX drift_zone_history_at ON drift_zone_history (at);

CREATE TABLE drift_group_reports (
    group_name TEXT PRIMARY KEY,
    last_success TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    report_json TEXT NOT NULL
);
