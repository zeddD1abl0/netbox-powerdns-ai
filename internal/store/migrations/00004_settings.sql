-- Runtime settings (ADR-0040): the values that `nbpdns settings set` stores,
-- by key. Each change bumps the settings_version sequence, which a running
-- serve watches.
-- +goose Up
CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by TEXT NOT NULL
);
INSERT INTO sequences (name, value) VALUES ('settings_version', 0);
