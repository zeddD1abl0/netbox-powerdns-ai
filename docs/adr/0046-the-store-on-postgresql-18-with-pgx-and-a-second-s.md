---
title: '0046: The store on PostgreSQL 18, with pgx and a second sqlc package'
status: accepted # proposed | accepted | rejected | deprecated | superseded by ADR-NNNN
date: 2026-10-18
decision-makers: [jordan]
requirements: [REQ-032, REQ-054]
questions: [Q-060, Q-061]
supersedes: # ADR-NNNN this replaces, if any
---

# 0046: The store on PostgreSQL 18, with pgx and a second sqlc package

## Context and problem statement

ADR-0009 chose PostgreSQL for several replicas, and SQLite for the single
binary, from one schema and one set of migrations. ADR-0038 built the
SQLite store, with goose's migrations and sqlc's queries, and wrote its SQL
so that PostgreSQL takes it: `sqlc compile` checks that. M09 runs the store
on PostgreSQL 18 (REQ-054).

Generated for PostgreSQL from today's SQL, sqlc's package differs from
SQLite's only in its placeholders, and in `LIMIT`'s parameters, `int32`
rather than `int64`. But the store relies on SQLite's immediate
transactions, which run one at a time. The audit trail reads its last
event, then appends the next one (ADR-0039). On PostgreSQL, two writers
could do that at once, and one would fail. Text also sorts by the
database's collation on PostgreSQL, and byte by byte on SQLite.

`database.url` must take PostgreSQL's standard form (the user, 2026-10-14),
which can hold a password.

## Decision drivers

- One schema, one set of migrations, one set of queries (REQ-032).
- Static builds, with CGO off (ADR-0009).
- Established libraries over custom code.
- The same answers from both databases, and the audit chain's guarantees on
  both.
- PostgreSQL's standard URI, as libpq and its tools read it.
- Secrets never logged or shown, and each with a `_FILE` variant.

## Considered options

1. **Driver:** jackc/pgx v5, through its `database/sql` adapter; pgx's own
   interface; lib/pq.
2. **Queries:** a second sqlc package behind an interface; SQLite's package
   alone, its placeholders rewritten for PostgreSQL as each query runs.
3. **Concurrent writes:** a transaction-scoped advisory lock first in each
   write transaction; `SERIALIZABLE` transactions, retried on conflict.
4. **`sslmode`'s default:** libpq's, `prefer`; `verify-full`; none, an
   `sslmode` required in the URL.
5. **The password from a file:** the whole URL from a file, with libpq's
   `passfile`; also a `database.password` key.

## Decision outcome

Chosen: **pgx through `database/sql`, a second sqlc package, an advisory
lock for writes, libpq's default `sslmode`, and the URL from a file**. The
user chose the second sqlc package on 2026-10-09 (ADR-0038), and
`sslmode`'s default and the password's file on 2026-10-18.

- **Driver:** pgx v5 (MIT), through its `database/sql` adapter, so that
  both dialects use `database/sql`, and sqlc's `database/sql` code.
- **Queries:** sqlc generates `internal/store/postgres` from the same SQL.
  `sqlc-postgres.yaml` changes from `compile` to `generate`, and
  `generate-check` diffs both packages. An override maps PostgreSQL's
  `int4` to `int64`, so that the two packages' structs have the same
  fields. The store calls a `querier` interface in the SQLite package's
  types; an adapter converts the PostgreSQL package's, and the compiler
  checks each conversion.
- **Migrations:** the same goose files, with goose's PostgreSQL dialect and
  its session locker, an advisory lock, so that replicas that start
  together migrate once.
- **Writes, one at a time:** on PostgreSQL, each write transaction first
  takes a transaction-scoped advisory lock, as SQLite's immediate
  transactions wait for each other. The audit chain, the sequences, and
  ADR-0041's rotation and ADR-0045's rename keep their guarantees.
- **Order:** the store sorts names in Go, byte by byte, so that lists come
  in the same order on both databases, whatever the collation. Times are
  stored at a fixed width (ADR-0038), so they compare correctly as text in
  any collation.
- **`database.url`:** `postgres://` or `postgresql://`, PostgreSQL's
  standard URI. pgx reads libpq's parameters and defaults: `sslmode`,
  `prefer` by default; `sslrootcert`, `sslcert`, `sslkey` and
  `sslpassword`, for TLS and client certificates; `passfile`; and a Unix
  socket through `host=`. It reads libpq's `PG*` environment variables too,
  under the URL's own values.
- **The URL is a secret:** it can come from `NBPDNS_DATABASE_URL_FILE`, and
  nbpdns never logs or shows its password. `/status` shows the URL without
  it, and whether the connection uses TLS. The docs recommend
  `sslmode=verify-full`.
- **Poolers:** nbpdns connects directly, or through a pooler in session
  mode. ADR-0047's lock, and ADR-0048's `LISTEN`, need a session.

### Consequences

- Good: every query is generated and type-checked for each dialect, from
  one SQL file, and the adapter's conversions fail to compile if the two
  packages drift apart.
- Good: the audit chain and every M08 guarantee hold on PostgreSQL as on
  SQLite, without retries.
- Good: an operator writes the URL as for `psql`, with the same TLS and
  password options.
- Bad: two generated packages, and an adapter of about 50 methods, kept in
  step by `generate-check` and the compiler.
- Bad: writes run one at a time on PostgreSQL too. nbpdns writes little,
  so the lock is held briefly, but it caps concurrent writes.
- Bad: `prefer` falls back to no encryption, and checks no certificate,
  unless the URL asks for more. The docs and `/status` make the mode
  visible.
- Bad: libpq's `PG*` variables can change the connection outside nbpdns's
  own configuration.

### Confirmation

- `generate-check` fails when either package is stale.
- The store's tests run on SQLite in `make test`, and on PostgreSQL 18, in
  the lab, in `make test-integration`. A test runs concurrent writers, and
  verifies the audit chain they leave.
- A test checks that the URL's password is in no log line, error or
  `/status` field.

## Pros and cons of the options

### pgx's own interface

- Good: pgx's full feature set, and its fastest path.
- Bad: sqlc's pgx code uses pgx's types, so the two packages would differ,
  and the store would need two code paths.

### lib/pq

- Good: long established.
- Bad: in maintenance mode; its own README points to pgx.

### SQLite's package alone, with rewritten placeholders

- Good: one package, and no adapter.
- Bad: custom code that rewrites SQL at run time, and the PostgreSQL
  queries' types aren't checked by sqlc.

### `SERIALIZABLE` transactions, retried

- Good: concurrent writes where they don't conflict.
- Bad: every write path needs a retry, and the audit trail's appends
  always conflict.

### `verify-full` by default

- Good: secure by default.
- Bad: differs from what `psql` and libpq do with the same URL.

### `sslmode` required

- Good: every install chooses.
- Bad: a URL that works with `psql` fails with nbpdns.

### A `database.password` key

- Good: a URL in a config map, and the password alone in a secret.
- Bad: a second way to give the password; libpq's `passfile` already
  serves that case.

## More information

- ADR-0009 (persistence and HA), ADR-0038 (the SQLite store), ADR-0039
  (the audit trail), ADR-0047 (the leader), ADR-0048 (shared state).
- Recorded in M09's design, 2026-10-18, by ITEM-0109. Implemented by
  ITEM-0110 and ITEM-0111.
