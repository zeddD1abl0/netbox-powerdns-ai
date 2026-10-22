-- The leader of the replicas of nbpdns serve on PostgreSQL (ADR-0047): one
-- row, which the replica that holds the leader's lock writes when it takes
-- over, and updates at each heartbeat. replica is its host's name, and
-- instance_id a random ID from its start. An empty next_refresh means that
-- no refresh is scheduled. On SQLite, where serve always leads, it stays
-- empty.
-- +goose Up
CREATE TABLE leader (
    singleton BIGINT PRIMARY KEY CHECK (singleton = 1),
    replica TEXT NOT NULL,
    instance_id TEXT NOT NULL,
    since TEXT NOT NULL,
    last_seen TEXT NOT NULL,
    next_refresh TEXT NOT NULL
);
