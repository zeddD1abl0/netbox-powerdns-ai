---
id: ITEM-0015
title: Split CI into stages
type: task
status: done
milestone: M00
requirements: [REQ-026, REQ-039]
depends_on: [ITEM-0014]
created: 2026-09-25
closed: 2026-09-25
---

# ITEM-0015: Split CI into stages

## Goal

Replace the single `make ci` job with staged pipelines on both forges: lint,
test, build and security, with room for security scanning, SBOMs and more
(REQ-039). Every job still runs make targets only. `make ci` stays the local
"run everything" target, and a lint rule stops the forges and `make ci` from
drifting apart (ADR-0016).

## Acceptance criteria

- [x] `.gitlab-ci.yml` has stages `lint`, `test`, `build` and `security`, with eight jobs. Each extends a hidden `.go` job that sets the pinned image and `GOTOOLCHAIN=local`.
- [x] `.github/workflows/ci.yml` has the same eight jobs, ordered with `needs:`.
- [x] `projctl lint` checks that each CI file's jobs run exactly the work targets of `make ci`. It lists anything missing or extra.
- [x] `projctl lint` checks that every CI image equals the Makefile's `CI_IMAGE`, and that each CI file names one.
- [x] Table-driven tests cover both rules; there are 49 subtests. Dropping the `vuln` job from `.gitlab-ci.yml` fails `make project-lint`.
- [x] Each job, run cold in a fresh pinned-image container, passes in under 200 MB and under 15 seconds.
- [x] ADR-0016 records the design. `CLAUDE.md`, the CHANGELOG and M00 are updated.

## Notes

- 2026-09-25: The coverage rule first compared every reachable target. It
  demanded that CI "run" the recipe-less aggregates `ci` and `check`, which
  split jobs never do. It now compares only targets with a recipe (the
  "work" targets), so running an aggregate's parts in separate jobs counts
  the same as running the aggregate.
- 2026-09-25: Each job was run cold in its own container (the pinned image,
  a non-root user, empty caches), as a GitLab pod would run it:

  | Job | Make targets | Disk | Time |
  |---|---|---|---|
  | go-lint | `vet lint` | 110 MB | 14 s |
  | docs-lint | `docs-lint` | 45 MB | 2 s |
  | api-lint | `api-lint` | 64 MB | 3 s |
  | project-lint | `project-lint` | 43 MB | 6 s |
  | unit-test | `test` | 72 MB | 11 s |
  | docs-site | `docs-links` | 64 MB | 7 s |
  | vuln | `vuln` | 182 MB | 13 s |
  | secrets | `secrets` | 22 MB | 3 s |

  The single job it replaces needed about 6.5 GB and 3.5 minutes. On the first
  run, `docs-lint` and `project-lint` failed on real problems in the working
  tree: the ADR-0016 title needed code formatting for Vale, and the board was
  stale. Both passed after the fixes.
- 2026-09-25: The GitLab `cache:` block is gone, because jobs are light and it
  depended on a shared runner cache.
