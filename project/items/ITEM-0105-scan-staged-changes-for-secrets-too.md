---
id: ITEM-0105
title: Scan staged changes for secrets too
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: []
depends_on: []
created: 2026-10-15
closed: 2026-10-16
---

# ITEM-0105: Scan staged changes for secrets too

## Goal

`make secrets`, and so `make check`, scan only the committed history with
gitleaks. A test fixture that looks like a secret therefore passes
`make check` and fails once it's committed, as ITEM-0104 found with commit
6544d0e, and a finding in history can then only be ignored, never removed.
Scanning the staged changes too catches it before the commit.

## Acceptance criteria

- [x] `make secrets` also scans the staged changes, and fails on a staged
  file that holds a secret gitleaks reports.
- [x] It still scans the whole history, as CI's `secrets` job needs.
- [x] `make`'s help says what it scans.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-15: Created from ITEM-0104's open question, which the user
  answered on 2026-10-15: `make secrets` should scan staged changes too.
- 2026-10-16: What gitleaks 8.30.1 scans, tested in a scratch repository:
  `git --staged` runs `git diff --staged`, the staged changes;
  `git --pre-commit` runs `git diff`, the unstaged changes to tracked
  files, but not untracked files; both flags together scan only the staged
  changes. With nothing to scan, each exits 0, as on CI's clean checkout.
  A finding that isn't committed has the fingerprint `file:rule:line`,
  which in `.gitleaksignore` also matches that line in every commit.
- 2026-10-16: `make secrets` runs three scans, all of them even when one
  fails, and names the ones that failed: the history, as before; the
  staged changes (`--staged`); and the unstaged and untracked changes
  (`--pre-commit`, with `GIT_INDEX_FILE` set to a throwaway copy of the
  index in which each untracked, not ignored file is marked intent-to-add).
  The real index isn't touched. Since the third scan covers what isn't
  staged yet, `make check` before `git add` still sees new files, and the
  workflow doesn't change. CI's jobs don't change either: they still need
  the full clone for the history, and a clean checkout adds two empty
  scans, under a second together.
- 2026-10-16: The user agreed: a finding that isn't committed yet is fixed
  by changing the value, or, if it must stay, by a `gitleaks:allow`
  comment on its line; its `file:rule:line` fingerprint only as a last
  resort. `.gitleaksignore`'s header says so.
- 2026-10-16: Checked in a scratch clone with the full history: it fails on
  a staged file, an unstaged edit and an untracked file holding a secret,
  and on a new commit holding one; it passes on a clean tree, scanning all
  156 commits, on a line marked `gitleaks:allow`, and on an ignored `.env`.
