---
id: ITEM-0017
title: Run the Claude Code hook pipe-tests in make test
type: debt # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M01
requirements: [REQ-022]
depends_on: [ITEM-0019]
created: 2026-09-26
closed: 2026-09-29
---

# ITEM-0017: Run the Claude Code hook pipe-tests in make test

## Goal

The main-branch guard (`.claude/hooks/guard-main-commit.sh`) and the goimports
edit hook in `.claude/settings.json` are tested only by hand-run pipe-tests,
recorded in item notes. Two code reviews in a row found ways past the guard
(ITEM-0013, ITEM-0016). Running those cases in `make test` would catch a
regression in CI, before a review has to.

## Acceptance criteria

- [x] A table-driven test pipes hook input into the guard against a scratch repo, on `main` and on a milestone branch. It covers at least the 32 cases in ITEM-0016's notes.
- [x] A test pipes the edit hook's command, read from `.claude/settings.json`, over a Go file that mixes third-party and internal imports, and checks the result passes the repo's golangci-lint formatters.
- [x] The tests run in `make test`, locally and in the CI image. If the CI image has no `jq`, the guard's fallback path is covered there.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-09-26: Found while fixing ITEM-0016. A hand-run version of the guard
  cases is in that item's notes. Where the tests live (`tools/projctl`, or a
  small module of their own) is decided in M01 design.
- 2026-09-29: Decided with the user: the tests live in their own Go module,
  `tools/hooktest`, added to the Makefile's `GO_MODULES`. `jq` is pinned as a
  release binary in `tools/tools.mk`, so the edit-hook test also runs in the
  CI image, which has no `jq`. Waits for ITEM-0019 so the board follows the
  phase order. The review found that, by design, the guard allows moving
  `main` without a commit (`git fetch . HEAD:main`, `git branch -f main`,
  `git update-ref`) and committing in a worktree on `main`; the tests should
  record that behavior explicitly.
- 2026-09-29: Done. `tools/hooktest` is a Go module with tests only, and
  `make test` runs it. The tests call each hook through the command
  registered in `.claude/settings.json`, via `/bin/sh -c` as Claude Code
  does, so the wiring is tested too.
  - **Guard:** 40 cases (24 denials, 16 allows) on `main` and on a milestone
    branch. They cover every category in ITEM-0016's notes, and record the
    by-design gaps (moving `main` without a commit, a worktree on `main`).
    Every case runs twice: with the pinned `jq`, and without `jq` on `PATH`.
  - **The tests found a bug.** Without `jq`, the guard matched the raw JSON,
    so a quoted argument (`git -C "a b" commit`), a command on a second
    line, or a tab after `git` got through on `main`. The fallback now undoes
    the JSON escapes first. Against the old guard, exactly those three cases
    fail.
  - **Edit hook:** run on a Go file that mixes standard, third-party and
    internal imports, in a path with a space. The result matches the
    expected grouping, and `golangci-lint fmt --diff` then reports nothing.
    The test first checks that golangci-lint does flag the unformatted input,
    so the check can't pass vacuously. Non-Go files are left alone.
  - **jq** 1.8.2 is pinned in `tools/tools.mk`. That needed two additions,
    both covered by `make tools-audit`: a per-tool release tag (`JQ_TAG`,
    since jq tags are `jq-1.8.2`), and fetching an asset that is itself the
    binary (`MEMBER` of `-`), which ITEM-0022 also needs for docker-compose.
  - `make test` passes on the host and in the pinned CI image, which has no
    `jq` and uses dash as `/bin/sh`.
