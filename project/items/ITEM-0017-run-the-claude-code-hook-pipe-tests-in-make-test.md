---
id: ITEM-0017
title: Run the Claude Code hook pipe-tests in make test
type: debt # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M01
requirements: [REQ-022]
depends_on: []
created: 2026-09-26
closed:
---

# ITEM-0017: Run the Claude Code hook pipe-tests in make test

## Goal

The main-branch guard (`.claude/hooks/guard-main-commit.sh`) and the goimports
edit hook in `.claude/settings.json` are tested only by hand-run pipe-tests,
recorded in item notes. Two code reviews in a row found ways past the guard
(ITEM-0013, ITEM-0016). Running those cases in `make test` would catch a
regression in CI, before a review has to.

## Acceptance criteria

- [ ] A table-driven test pipes hook input into the guard against a scratch repo, on `main` and on a milestone branch. It covers at least the 32 cases in ITEM-0016's notes.
- [ ] A test pipes the edit hook's command, read from `.claude/settings.json`, over a Go file that mixes third-party and internal imports, and checks the result passes the repo's golangci-lint formatters.
- [ ] The tests run in `make test`, locally and in the CI image. If the CI image has no `jq`, the guard's fallback path is covered there.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-09-26: Found while fixing ITEM-0016. A hand-run version of the guard
  cases is in that item's notes. Where the tests live (`tools/projctl`, or a
  small module of their own) is decided in M01 design.
