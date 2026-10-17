---
id: ITEM-0108
title: Say one secret, not 1 secrets, after a master key's rotation
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-051]
depends_on: []
created: 2026-10-17
closed: 2026-10-17
---

# ITEM-0108: Say one secret, not 1 secrets, after a master key's rotation

## Goal

M08's manual verification rotated a master key over the one secret that the
database stored, and `nbpdns secrets rotate-master-key` said "Re-wrapped 1
secrets". The how-to quotes the same output.

## Acceptance criteria

- [x] `rotate-master-key` says "1 secret" for one, and "secrets" for any
  other count.
- [x] Its tests, and the how-to's example output, say it the same way.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-17: Found in M08's manual verification, step 5 (see M08's
  verification log). Fixed in `internal/cli/secrets.go`; the tests in
  `secrets_test.go` and `groups_test.go`, and "Protect stored secrets with
  a master key", follow. Unreleased, so no CHANGELOG line.
