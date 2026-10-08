---
id: ITEM-0075
title: Run the integration tests apart from the unit tests
type: task # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M07
requirements: [REQ-039]
depends_on: []
created: 2026-10-08
closed: 2026-10-09
---

# ITEM-0075: Run the integration tests apart from the unit tests

## Goal

GitLab's runner has killed the integration job under memory pressure on
two pipelines (jobs 4253 and 4288). `unit-test` and `integration-test` run
at once in stage `test`, and `make test-integration` reruns every unit test
of every module with `-race`, though only four packages have integration
tests. Following the user's suggestion, the integration job runs apart
from the unit tests, and only what it needs.

## Acceptance criteria

- [x] `make test-integration` runs only the packages that have integration-tagged tests, found by their build tag.
- [x] Both forges' `integration-test` jobs wait for `unit-test`, and `make project-lint` passes.
- [x] The first pipeline on the branch records both jobs' times, and whether the integration job was killed.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M07's approved design.
- 2026-10-08: `make test-integration` now picks, in each module, the
  packages whose test files change with `-tags integration` (comparing
  `go list`'s test files with and without the tag), and fails if none has
  any. It finds internal/cli, internal/lab, internal/netbox and
  internal/powerdns, and skips tools/projctl and tools/hooktest, so it no
  longer needs the hook tests' jq and golangci-lint. Both forges'
  `integration-test` jobs have `needs: [unit-test]`; on GitLab the job
  stays in stage `test`, so the build stage still waits for it.
  `make project-lint` passes. Locally, uncached (`GOFLAGS=-count=1`), with
  a warm build cache: 42.8 s, against 46.4 s for the old command, and the
  same peak per process, 1.27 GB. The gain on the runner, a cold build and
  no unit tests beside it, shows only in a pipeline: the third criterion
  waits for the user to push the branch.
- 2026-10-09: The branch's first pipelines, on 0a78faa, passed without a
  retry. GitLab pipeline 786: `unit-test` (job 4299) ran 22:49:47 to
  23:00:04, 616.5 s, and `integration-test` (job 4300) started at
  23:00:05, a second later, and passed in 1,409.9 s; nothing was killed.
  Before, the two started together, and the integration job failed on its
  first try in four of the last five pipelines (jobs 4229, 4253, 4276 and
  4288), passing on retry in 1,413 to 1,430 s, when it ran alone. So
  running it alone takes as long, and no longer fails; the narrower package
  list saves little on the runner, where the lab's start dominates. The
  pipeline took 51 m 52 s in all. GitHub run 37778553208 passed on its
  first attempt: `unit-test` 12:42:53 to 12:46:30, `integration-test`
  12:46:33 to 12:54:04.
- 2026-10-09: Seen, not this item's: GitLab's `unit-test` has taken about
  620 s since pipeline 782 (M06), against about 225 s before.
