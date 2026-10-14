---
id: ITEM-0105
title: Scan staged changes for secrets too
type: bug # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: []
depends_on: []
created: 2026-10-15
closed:
---

# ITEM-0105: Scan staged changes for secrets too

## Goal

`make secrets`, and so `make check`, scan only the committed history with
gitleaks. A test fixture that looks like a secret therefore passes
`make check` and fails once it's committed, as ITEM-0104 found with commit
6544d0e, and a finding in history can then only be ignored, never removed.
Scanning the staged changes too catches it before the commit.

## Acceptance criteria

- [ ] `make secrets` also scans the staged changes, and fails on a staged
  file that holds a secret gitleaks reports.
- [ ] It still scans the whole history, as CI's `secrets` job needs.
- [ ] `make`'s help says what it scans.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-15: Created from ITEM-0104's open question, which the user
  answered on 2026-10-15: `make secrets` should scan staged changes too.
