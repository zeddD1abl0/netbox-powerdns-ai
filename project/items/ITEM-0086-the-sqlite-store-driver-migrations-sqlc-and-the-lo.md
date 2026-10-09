---
id: ITEM-0086
title: 'The SQLite store: driver, migrations, sqlc and the lock'
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-032, REQ-033, REQ-050]
depends_on: []
created: 2026-10-09
closed:
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

- [ ] `serve` opens or creates the database at `database.url`, applies its migrations, and fails clearly when it can't.
- [ ] A second `serve` on the same database refuses to start, naming the holder's host and PID.
- [ ] sqlc runs in `make generate`, and `generate-check` and a portability test guard the SQL.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-09: Created from M08's approved design, on `plan-m08-m12`.
