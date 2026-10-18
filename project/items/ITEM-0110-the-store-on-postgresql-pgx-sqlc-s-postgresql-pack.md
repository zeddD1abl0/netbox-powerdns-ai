---
id: ITEM-0110
title: 'The store on PostgreSQL: pgx, sqlc''s PostgreSQL package and tests on both'
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: [REQ-032, REQ-054]
depends_on: [ITEM-0109]
created: 2026-10-18
closed: 2026-10-18
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

- [x] `make generate` writes both packages, and `generate-check` fails when either is stale.
- [x] Every store test passes on SQLite in `make test`, and on PostgreSQL 18 in `make test-integration`.
- [x] Concurrent writers on PostgreSQL all append to the audit trail, and `nbpdns audit verify`'s check passes on it.
- [x] Lists come in the same byte order on both databases, whatever the collation.
- [x] Replicas migrating one database at once apply each migration once.
- [x] `make lab-up` creates the `nbpdns` role and database on an existing volume, and does nothing when they exist.
- [ ] The integration job's peak memory on the first pipeline is recorded here, for ITEM-0100.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-18: Created from M09's approved design, at M09's start. The pgx dependency, and its
  modules, are justified here when they're added.
- 2026-10-18: Dependencies. jackc/pgx v5.11.0 (MIT, released 2026-09-07),
  through `pgx/v5/stdlib`, as ADR-0046 chose: lib/pq is in maintenance
  mode, and pgx's own interface would give the two sqlc packages different
  types. pgx v5.10.0 was in the module graph already, through goose's
  `go.mod`, but unused. The modules it brings, all MIT, by pgx's author:
  jackc/pgpassfile v1.0.0, libpq's `passfile`, which ITEM-0111's password
  from a file needs; jackc/pgservicefile (2024-06-06), libpq's service
  file, which pgconn reads for `service=`; and jackc/puddle/v2 v2.2.2,
  pgxpool's pool, which `stdlib` imports for `OpenDBFromPool`, so it's
  linked though nbpdns calls `stdlib.OpenDB`. pgx also uses x/sync's
  semaphore and x/text's precis (SCRAM's SASLprep), BSD-3-Clause, from
  modules already in the build. `make vuln` finds nothing new: only
  GO-2026-5932, in x/crypto, which the code doesn't call.
- 2026-10-18: Built.
  - sqlc: `emit_interface` on `sqlc.yaml` generates `sqlite.Querier`, the
    interface in the SQLite package's types. `sqlc-postgres.yaml` generates
    `internal/store/postgres`, with an override of `integer` to `int64`:
    sqlc names LIMIT's parameters' type `integer`, and an override of
    `pg_catalog.int4` didn't reach them. portable_test refuses `INTEGER`
    columns, so there's no other int4. `make generate` writes both
    packages, and `generate-check` runs `sqlc diff` on both (checked: a
    hand edit to `postgres/` fails it).
  - The adapter, `pgqueries.go`, implements the interface's 49 methods with
    Go struct conversions, which the compiler checks; `var _` asserts that
    both packages implement it.
  - `ListAuditEvents`' optional filters, `narg IS NULL OR …`, failed on
    PostgreSQL with 42P08, since `IS NULL` doesn't type a parameter.
    `CAST(narg AS TEXT) IS NULL` types it on both, and sqlc now gives the
    filters `sql.NullString` rather than `interface{}`.
  - Order: the five queries that sorted by a name, by the database's
    collation, no longer sort. The store sorts in Go, byte by byte, where
    the order shows: the groups (`Groups`, and `MirrorFileGroups`'
    removals, which become events in that order) and the sealed settings
    (which value an error names first). The other three fill maps.
  - Writes on PostgreSQL: `InTx` begins read committed, whatever
    `default_transaction_isolation` says, so that each statement after the
    lock sees the writes committed before it; sets `lock_timeout` to
    `busyTimeout`, 5 s, as SQLite's busy timeout; and takes
    `pg_advisory_xact_lock` on a constant key, since advisory locks belong
    to their database. Reads begin read only and repeatable read, so that
    `VerifyAudit`'s pages read one snapshot, as a SQLite read transaction
    does in WAL mode; modernc ignores the level.
  - Migrations: goose's PostgreSQL dialect, and its session locker, which
    tries every 1 s, for 5 minutes, rather than every 5 s. Each provider
    gets a new locker, since go-retry's retry count lasts across calls.
  - `OpenPostgres(ctx, url)`: `pgx.ParseConfig`, whose errors hide the
    password, `stdlib.OpenDB`, and a ping. `Path()` is empty on
    PostgreSQL. ITEM-0111 makes `Open` choose by `database.url`.
  - Tests: a backend list, SQLite, which `postgres_integration_test.go`
    extends with the lab's PostgreSQL; `onEach` runs each store test on
    each. `lab.NewPostgresDatabase` creates a database owned by `nbpdns`,
    as the superuser, and drops it `WITH (FORCE)` after the test. The old
    `TestConcurrentWriters` counted in a raw transaction, outside `InTx`;
    it now appends audit events from two stores, two writers each, and
    verifies the chain. `TestConcurrentMigrations` (PostgreSQL only) runs
    four at once. `TestGroupsInByteOrder` fails if the database's collation
    sorts byte by byte, since it would prove nothing. Each fails without
    its guard (checked): a duplicate `audit_events_pkey` without the write
    lock, a duplicate type on migrating without goose's locker, and the
    insertion order without the sort.
  - The lab: postgres-47 publishes 5418 on the bind address; `make lab-up`
    pipes `deploy/dev/nbpdns-database.sql` to psql, whose `\gexec` creates
    the role and the database only if they're missing (checked on the
    existing volume, and again, which prints nothing). The server's
    collation is `en_US.utf8`. A moved line in `settings_test.go` lost
    ITEM-0104's gitleaks fingerprint, so its fake webhook secret is now
    low-entropy text.
- 2026-10-18: Memory, for ITEM-0100, measured as it measured: `make
  test-integration` with an empty build cache, on 16 CPUs, in a scope of
  its own, its anonymous memory sampled every 100 ms. The job's processes
  peak at 1.68 GiB, against ITEM-0100's 1.65 GiB for the integration
  build alone, inside `.heavy`'s 2 GiB request, in 201 s. The lab's
  PostgreSQL, anonymous and shared memory, goes from 0.08 to 0.11 GiB
  while the store's tests create and drop their databases, inside the
  service's 1536 MiB. The CI job doesn't report its own peak, and CI files
  only call make targets, so the last criterion stays open until the first
  pipeline runs, once the user pushes the branch: the next session records
  whether its integration job passes without a retry, here and in
  ITEM-0100. Everything else is done: `make check` and `make
  test-integration` pass.
