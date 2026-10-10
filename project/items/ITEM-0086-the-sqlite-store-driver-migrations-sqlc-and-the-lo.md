---
id: ITEM-0086
title: 'The SQLite store: driver, migrations, sqlc and the lock'
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-032, REQ-033, REQ-050]
depends_on: []
created: 2026-10-09
closed: 2026-10-10
---

# ITEM-0086: The SQLite store: driver, migrations, sqlc and the lock

## Goal

The database under everything else in M08 (ADR-0038): modernc.org/sqlite
in WAL mode with a busy timeout; goose's embedded, forward-only migrations,
applied at `serve`'s start, with `nbpdns db status` and `nbpdns db
migrate`; sqlc, a pinned release binary, generating `internal/store/sqlite`
from portable queries; `database.url`, a bootstrap key; and the
single-instance `flock`.

## Acceptance criteria

- [x] `serve` opens or creates the database at `database.url`, applies its migrations, and fails clearly when it can't.
- [x] A second `serve` on the same database refuses to start, naming the holder's host and PID.
- [x] sqlc runs in `make generate`, and `generate-check` and a portability test guard the SQL.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-09: Created from M08's approved design, on `plan-m08-m12`.
- 2026-10-10: Dependencies, each justified by ADR-0038:
  modernc.org/sqlite v1.60.1 (BSD-3-Clause), the pure-Go driver, and
  github.com/pressly/goose/v3 v3.28.0 (MIT), the migrations, with their
  modules: modernc.org/libc, mathutil and memory, remyoudompheng/bigfft
  (BSD-3-Clause), dustin/go-humanize, ncruces/go-strftime, mattn/go-isatty,
  mfridman/interpolate and go.uber.org/multierr (MIT), and
  sethvargo/go-retry (Apache-2.0). sqlc v1.31.1 is a tool, pinned in
  `tools/tools.mk`, not a dependency.
- 2026-10-10: Built `internal/store`: `Open` (WAL, a 5 s busy timeout,
  synchronous FULL so that a committed audit event survives a power cut,
  foreign keys, immediate transactions; the file made 0600 before SQLite
  opens it, and the directory checked, not created, so a missing volume
  fails clearly); `Migrate` and `Status`, on goose's provider with the
  migrations embedded, refusing a database newer than the binary, which
  goose itself accepts silently; `Acquire`, the flock on `<db>.lock`,
  naming its holder, with control characters stripped; and the first
  migration, `database_info`, the database's random ID, creation time and
  version. `nbpdns db status` and `nbpdns db migrate`; `db migrate` takes
  the lock, since a schema change under an older running serve would
  break it. `serve` takes the lock, opens, migrates and logs the database
  before it makes any client.
- 2026-10-10: Portability (ADR-0038): `sqlc compile` on
  `internal/store/sqlc-postgres.yaml`, which has a `gen` block only so that
  compile checks it (without one, compile skips the entry), runs in `make
  generate` and `generate-check`; `generate-check` also runs `sqlc diff`.
  Checked both by breaking them: a stale query and a `GLOB` each fail it.
  `TestSQLIsPortable` refuses SQLite-only types and functions, which sqlc's
  PostgreSQL engine lets through, and `TestMigrationsOnlyMoveForward`
  refuses a Down section or a gap in the numbers. sqlc's releases publish
  no checksums file, so `tools/update.sh` now takes a release's asset
  digests from GitHub's API when `_CHECKSUMS` is empty; `make tools-audit`
  passes with it.
- 2026-10-10: `serve` needs a writable directory now, so the tutorial "Run
  nbpdns as a service" sets `database.url: sqlite:nbpdns.db`, "Install
  nbpdns" makes `/var/lib/nbpdns`, and "Run nbpdns in a container" mounts a
  volume there: a bind mount owned by 65532 for Docker, and a PVC for
  Kubernetes, with the `Recreate` strategy, since a rolling update would
  wait forever for a pod that can't take the lock. ITEM-0093 adds the image
  check with a volume, and ITEM-0094 the explanation. Vale ignores
  configuration keys, such as `database.url`, which the command-line
  reference shows as the help writes them.
- 2026-10-10: Done. `make check` passed (govulncheck's download timed out once,
  and passed when run again); `make test-integration` passed (internal/cli
  60.7 s, internal/netbox 9.7 s); `make release-check` passed, with the
  driver in both architectures' binaries.
