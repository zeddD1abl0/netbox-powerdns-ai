---
id: ITEM-0075
title: Run the integration tests apart from the unit tests
type: task # feature | bug | debt | task
status: in-progress # open | in-progress | blocked | done | wontfix
milestone: M07
requirements: [REQ-039]
depends_on: []
created: 2026-10-08
closed:
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
- [ ] The first pipeline on the branch records both jobs' times, and whether the integration job was killed.

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
