---
id: ITEM-0110
title: 'The store on PostgreSQL: pgx, sqlc''s PostgreSQL package and tests on both'
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: [REQ-032, REQ-054]
depends_on: [ITEM-0109]
created: 2026-10-18
closed:
---

# ITEM-0110: The store on PostgreSQL: pgx, sqlc's PostgreSQL package and tests on both

## Goal

The store on PostgreSQL 18 (ADR-0046): pgx v5 through `database/sql`;
sqlc's PostgreSQL package, `internal/store/postgres`, with an `int4` to
`int64` override, behind a `querier` interface and an adapter; goose's
PostgreSQL dialect, with its session locker; write transactions run one at
a time, behind a transaction-scoped advisory lock; names sorted in Go; the
store's tests on both databases, PostgreSQL's in the integration suite,
each in a fresh database; and the lab's `nbpdns` role and database, with
the database's port published.

## Acceptance criteria

- [ ] `make generate` writes both packages, and `generate-check` fails when either is stale.
- [ ] Every store test passes on SQLite in `make test`, and on PostgreSQL 18 in `make test-integration`.
- [ ] Concurrent writers on PostgreSQL all append to the audit trail, and `nbpdns audit verify`'s check passes on it.
- [ ] Lists come in the same byte order on both databases, whatever the collation.
- [ ] Replicas migrating one database at once apply each migration once.
- [ ] `make lab-up` creates the `nbpdns` role and database on an existing volume, and does nothing when they exist.
- [ ] The integration job's peak memory on the first pipeline is recorded here, for ITEM-0100.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-18: Created from M09's approved design, at M09's start. The pgx dependency, and its
  modules, are justified here when they're added.
