---
id: ITEM-0013
title: Fix the M0 code-review findings
type: bug
status: done
milestone: M00
requirements: [REQ-012, REQ-022, REQ-026, REQ-037]
depends_on: []
created: 2026-09-25
closed: 2026-09-25
---

# ITEM-0013: Fix the M0 code-review findings

## Goal

Fix the 10 findings from the `/code-review high` run at M0 close (ITEM-0011).
Each one was checked against the code. One of them would fail every GitLab
pipeline; the rest let a tracking, CI or lint rule be bypassed or silently
skipped.

## Acceptance criteria

- [x] `projctl lint` skips hidden directories except `.claude` and `.github`, so the CI cache in `.cache/` is never scanned. Covered by a test.
- [x] `projctl new` refuses to run while any tracking file fails to parse, as `index` already does. Covered by a test.
- [x] `projctl lint` reports duplicate ADR numbers. Covered by a test.
- [x] `docs-links` depends on `docs`, so `make -j` can't check a missing or half-built site.
- [x] The main-branch commit guard catches `git -c … commit`, `git --no-pager commit`, and `git switch main && git commit`, and doesn't depend on `jq`. Pipe-tested.
- [x] `zalando-176` also flags an error response that has no `content`. Checked against a fixture.
- [x] The CI-file check accepts only `make <target>…`, with no flags, variables or paths. It also rejects GitHub `uses:` other than `actions/checkout` pinned to a commit SHA, `shell:` overrides, GitLab `include:`, and GitLab hook scripts. Covered by tests.
- [x] `projctl new` quotes titles through the YAML encoder, so titles like `Decide:`, `null`, `true` and `123` round-trip. Covered by a test.
- [x] `each_module` fails when `go list` fails, rather than reporting "no packages yet".
- [x] `make item` and `make adr` pass `TITLE` through the environment, so quotes, backticks and `$(…)` are never run by the shell.
- [x] `make ci` is green.

## Notes

- 2026-09-25: Reproduced the `.cache` finding before fixing it. A Markdown file
  with a broken link under `.cache/probe/` made `make project-lint` fail. On
  GitLab, `GOMODCACHE` and `GOCACHE` live in `$CI_PROJECT_DIR/.cache`, so the
  first pipeline would have failed at `project-lint`. GitHub and the local
  container test keep caches outside the checkout, which is why neither caught
  it.
- 2026-09-25: All 10 fixed. The notable details:
  - `projctl`: its tests grew from 26 to 43 subtests. Two new cases guard the
    `.cache` fix: a file under `.cache/` is ignored, and one under `.claude/`
    is still checked.
  - The title round-trip test found another bug: a title with no letters or
    digits (such as `~`) gave an empty slug (`ITEM-0004-.md`). Such titles
    now get the slug `untitled`.
  - `make item` with a title containing quotes, `$(whoami)` and backticks
    stored it literally, in a scratch copy of the repo. Titles now reach
    projctl through the environment (`override TITLE := $(value TITLE)`), so
    make doesn't expand them and the shell doesn't run them.
  - `each_module` stopped on a deliberately broken `go.mod` ("unknown
    directive"). It used to report "no packages yet, skipped".
  - The commit guard was pipe-tested in 14 cases:
    - `-c`/`--no-pager`/`-C` forms and `switch|checkout main && commit` are
      denied;
    - `git log --grep commit`, `echo git committed` and
      `switch mainline-fix` are allowed;
    - without jq on PATH, a commit on main is still denied.
  - `zalando-176` reports both bad responses in `bad.yaml` (a bodiless 404 and
    a 418 with plain JSON).
