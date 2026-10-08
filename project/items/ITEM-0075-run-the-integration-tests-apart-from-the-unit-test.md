---
id: ITEM-0075
title: Run the integration tests apart from the unit tests
type: task # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
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

- [ ] `make test-integration` runs only the packages that have integration-tagged tests, found by their build tag.
- [ ] Both forges' `integration-test` jobs wait for `unit-test`, and `make project-lint` passes.
- [ ] The first pipeline on the branch records both jobs' times, and whether the integration job was killed.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M07's approved design.
