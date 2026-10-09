---
id: M09
title: PostgreSQL and HA
status: planned # planned | in-progress | done
started:
closed:
---

# M09: PostgreSQL and HA

A roadmap entry, written ahead on `plan-m08-m12` (ADR-0037). Its design,
items and ADRs come in a plan-mode session before it starts.

## Goal

Run several replicas of `nbpdns serve` on PostgreSQL, with exactly one of
them refreshing, and another taking over within about 30 seconds when it
stops (REQ-033).

## Scope

- **PostgreSQL 18**, with the same migrations and queries as SQLite
  (ADR-0009, ADR-0038): sqlc's second, PostgreSQL package, behind the store's
  interface, and the store's tests run against both databases. More releases
  are added as the CI runners have room, as for NetBox and PowerDNS.
- **The leader:** a PostgreSQL session advisory lock, held on a connection
  of its own. Only the leader runs refreshes, the prune and, from M13, the
  writes; a replica that gets the lock takes over. TCP keepalives bound how
  long a dead leader's lock lasts, to about 30 seconds.
- **Every replica serves the API and status from the database**, not from
  its memory, so that each answers the same.
- **Webhooks on any replica:** a replica that isn't the leader queues the
  zones in the database, for the leader to refresh. This lifts M07's "a
  webhook reaches one replica" limit.
- **Settings and groups changed with the CLI** reach every replica, through
  the settings' version, as in M08.
- **`/status` and the metrics** say which replica leads, and since when.
- **The lab:** a database for nbpdns in the lab's existing PostgreSQL 18,
  which NetBox already runs, so CI needs no more memory.

## Non-goals

- **No move from SQLite:** an install that changes to PostgreSQL starts with
  an empty database.
- **No HA on SQLite:** it stays a single instance (ADR-0009).
- **No PgBouncer in transaction mode** for the leader's connection, which
  the advisory lock can't survive; the docs say so.
- **No multi-region or read replicas.**

## Dependencies

- M08: the store, its migrations and queries, the audit trail, settings and
  groups in the database, and drift history.

## Decided, 2026-10-09

- PostgreSQL 18, more releases as the runners allow (REQ-054, Q-060).
- No move from SQLite to PostgreSQL (Q-061).
- Leader election by a PostgreSQL advisory lock (Q-062), as ADR-0009
  intended.

## Open for its design

- How the replicas learn of a change of leader quickly: `LISTEN` and
  `NOTIFY`, or polling.
- The replica tests: how a leader is killed in CI, and how long a takeover
  may take there.
- `database.url`'s form for PostgreSQL: TLS, client certificates, and the
  password from a `_FILE`.
