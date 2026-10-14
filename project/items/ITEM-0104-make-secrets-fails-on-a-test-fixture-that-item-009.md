---
id: ITEM-0104
title: Make secrets fails on a test fixture that ITEM-0092 committed
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: []
depends_on: []
created: 2026-10-15
closed: 2026-10-15
---

# ITEM-0104: Make secrets fails on a test fixture that ITEM-0092 committed

## Goal

`make secrets`, and so `make check`, pass again. Commit 6544d0e (ITEM-0092)
added a test's fake webhook secret, 16 hex digits, in
`internal/store/settings_test.go`, which gitleaks' `generic-api-key` rule
reports. The scan reads only the committed history, so `make check` didn't
see it before the commit. The finding can't be removed from history, so it's
ignored by its fingerprint, which matches that one finding and nothing else.

## Acceptance criteria

- [x] `.gitleaksignore` names the finding's fingerprint, and `make secrets`
  passes.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-15: Found while running `make check` for ITEM-0091. A fingerprint,
  rather than an allowlist regex for the value, so that a real secret that
  happens to match it still fails. `make secrets` scans only commits, so a
  fixture like this one passes `make check` and fails after its commit;
  scanning the staged changes too would catch it first, which is left to
  decide with the user.
- 2026-10-15: The user answered the open question: `make secrets` should
  scan the staged changes too. ITEM-0105 does it.
