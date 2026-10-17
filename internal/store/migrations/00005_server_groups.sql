-- Server groups (ADR-0040): the config file's, mirrored at serve's start as
-- managed_by 'file', and those that `nbpdns groups` adds, as managed_by
-- 'cli'. Each has an ID that nbpdns assigns and never reuses, which a CLI
-- group's sealed API key is bound to, so that a rename re-seals nothing
-- (ADR-0045). A file group's key stays in the file. spec_json holds the
-- group's other fields, as the config file names them. Each change bumps
-- the groups_version sequence, which a running serve watches.
-- +goose Up
CREATE TABLE server_groups (
    id BIGINT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    managed_by TEXT NOT NULL,
    spec_json TEXT NOT NULL,
    api_key TEXT NOT NULL,
    created_at TEXT NOT NULL,
    created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by TEXT NOT NULL
);
INSERT INTO sequences (name, value) VALUES ('server_groups', 0), ('groups_version', 0);
