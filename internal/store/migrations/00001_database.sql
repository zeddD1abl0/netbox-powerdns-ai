-- The database's identity: one row, written when nbpdns creates it.
-- +goose Up
CREATE TABLE database_info (
    singleton BIGINT PRIMARY KEY CHECK (singleton = 1),
    id TEXT NOT NULL,
    created_at TEXT NOT NULL,
    created_by TEXT NOT NULL
);
