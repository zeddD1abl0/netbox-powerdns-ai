---
title: '0038: An embedded SQLite store, with goose and sqlc'
status: proposed # proposed | accepted | rejected | deprecated | superseded by ADR-NNNN
date: 2026-10-09
decision-makers: [jordan]
requirements: [REQ-032, REQ-033, REQ-050]
questions: [Q-026]
supersedes: # ADR-NNNN this replaces, if any
---

# 0038: An embedded SQLite store, with goose and sqlc

## Context and problem statement

From M08, `nbpdns serve` keeps its state in a database: the audit trail,
runtime settings, server groups, stored secrets and drift history. ADR-0009
chose embedded SQLite for the single binary and PostgreSQL for HA, from one
schema and one set of migrations, with a pure-Go SQLite driver so that builds
stay static. It left the driver, the migrations and the query layer to a
design session. PostgreSQL comes in M09, so M08's SQL must already work on
both.

The image is built by ko on distroless, as user 65532 (ADR-0030, ADR-0031).
ko adds only the binary, so it can't create a directory in the image or
declare a volume.

## Decision drivers

- Static builds with CGO off (ADR-0009, REQ-045).
- One schema and one query set for SQLite and PostgreSQL (REQ-032).
- Established libraries over custom code.
- Generated code, checked in CI, as the API's is (ADR-0033).
- Upgrades that only move forward (Q-026).
- Exactly one `serve` on a SQLite database (REQ-033).

## Considered options

1. **Driver:** modernc.org/sqlite; ncruces/go-sqlite3, on WebAssembly;
   mattn/go-sqlite3, which needs CGO.
2. **Migrations:** goose; golang-migrate; Atlas.
3. **Queries:** sqlc; bun; sqlx.

## Decision outcome

Chosen: **modernc.org/sqlite, goose and sqlc**. The user chose sqlc on
2026-10-09.

- **Driver:** modernc.org/sqlite, opened in WAL mode with a busy timeout, so
  that the CLI can write while `serve` runs.
- **Migrations:** goose, with SQL files embedded in the binary, forward-only.
  `serve` applies pending migrations at start and logs them; `nbpdns db
  status` and `nbpdns db migrate` show and apply them. A test refuses SQL
  that PostgreSQL wouldn't take.
- **Queries:** sqlc generates type-checked Go from `internal/store/*.sql`
  into `internal/store/sqlite`, for database/sql. The queries use named
  parameters, so that M09 generates its PostgreSQL package from the same
  files. `make generate` runs sqlc, and `generate-check` keeps its output
  current. sqlc needs CGO to build, so it's a release binary pinned by
  SHA-256 in `tools/tools.mk`.
- **Location:** `database.url`, a bootstrap key, by default
  `sqlite:///var/lib/nbpdns/nbpdns.db`. `serve` needs it, and fails clearly
  if it can't open, create or lock the database. The one-shot commands, such
  as `nbpdns drift`, need none.
- **The image:** deployments mount `/var/lib/nbpdns`: Kubernetes with
  `fsGroup: 65532`, Docker with a bind mount owned by 65532. The release
  check runs the image that way, and writes the database.
- **One instance:** `serve` holds an exclusive `flock` on `<database>.lock`,
  and refuses to start while another `serve` holds it, naming the holder's
  host and PID. The kernel releases it if `serve` dies. The CLI doesn't take
  it.

### Consequences

- Good: the binary stays static, and needs nothing beside it.
- Good: every query is checked when it's generated, and M09's PostgreSQL code
  comes from the same SQL.
- Bad: sqlc generates a package per dialect, so M09 adds a second one behind
  an interface.
- Bad: forward-only migrations mean a downgrade needs a backup.
- Bad: the image can't carry its data directory, so each deployment mounts
  it.

### Confirmation

- `generate-check` fails when the generated queries are stale.
- A test applies every migration to an empty database and to one at each
  earlier version, and refuses SQLite-only syntax.
- An integration test starts a second `serve` on the same database, and sees
  it refuse.
- The release check writes the database in the image, on a volume.

## Pros and cons of the options

### ncruces/go-sqlite3

- Good: pure Go, through WebAssembly, and fast.
- Bad: younger, and a WebAssembly runtime in the binary.

### mattn/go-sqlite3

- Good: the reference driver.
- Bad: needs CGO, which ADR-0009 rules out.

### golang-migrate

- Good: widely used.
- Bad: needs a down file for each migration, which forward-only doesn't use.

### Atlas

- Good: declarative schemas, and diffs.
- Bad: much of it is a hosted product, and its tooling is larger than the
  problem.

### bun

- Good: one code path for both dialects.
- Bad: queries are checked only when they run.

### sqlx

- Good: the thinnest layer over database/sql.
- Bad: nothing checks the SQL until it runs, and it's little maintained.

## More information

- ADR-0009 (persistence and HA), ADR-0030 and ADR-0031 (the image), ADR-0033
  (generated API code).
- Recorded in M08's design, 2026-10-09, on `plan-m08-m12` (ADR-0037).
  Implemented by ITEM-0086.
