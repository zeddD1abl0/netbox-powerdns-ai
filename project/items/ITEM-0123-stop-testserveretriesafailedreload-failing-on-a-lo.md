---
id: ITEM-0123
title: Stop TestServeRetriesAFailedReload failing on a locked database
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: []
depends_on: []
created: 2026-11-06
closed: 2026-11-25
---

# ITEM-0123: Stop TestServeRetriesAFailedReload failing on a locked database

## Goal

`TestServeRetriesAFailedReload` (`internal/cli/settings_test.go`) edits a
running serve's SQLite database through a connection of its own, opened
with `sql.Open("sqlite", db)`, which has no `busy_timeout`. When serve
writes at the same moment, as it does at its start and at its first
refresh, the test's `INSERT` or `UPDATE` fails at once with `database is
locked (5) (SQLITE_BUSY)`, rather than waiting, and the test fails. It
fails `make check` now and then, and could fail CI's unit-test job. The
test's connection should wait for serve's writes, as the store's do.

## Acceptance criteria

- [x] The test's own connection waits for serve's writes, with the store's busy timeout.
- [x] `go test -race -count=30 -run TestServeRetriesAFailedReload ./internal/cli/` passes every time.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-11-06: Found in ITEM-0115's `make check`. It's older than
  ITEM-0115: with `-count=15`, it failed 3 times on 5ff6337, and 5 times
  with ITEM-0115's change, at lines 131 and 132, the test's first two
  writes. The store opens SQLite with `_pragma=busy_timeout(5000)`
  (`dsn`, `internal/store/store.go`); the test's connection could use the
  same pragma, or the store's own `Open`.
- 2026-11-25: Failed again in ITEM-0116's `make check`, at line 131,
  the test's first write, with `database is locked (5) (SQLITE_BUSY)`.
- 2026-11-25: Done. Before the fix, `-count=30` failed twice, at line 131,
  with SQLITE_BUSY. The three tests that edit a SQLite database behind
  nbpdns's back, `TestServeRetriesAFailedReload`,
  `TestServeRecordsItsStartAndStop` and `TestServeRestarts`, now open it
  with `rawSQLite` (`internal/cli/serve_test.go`), whose
  `_pragma=busy_timeout(5000)` waits as the store's `dsn` does; the store
  keeps `busyTimeout` unexported, so the helper's comment names it. The
  blank import of the driver moved there. After it, `-count=30` passed 30
  times, and `-count=60` on 2 CPUs (`taskset -c 0,1`) 60 times. No
  CHANGELOG line: only tests change.
