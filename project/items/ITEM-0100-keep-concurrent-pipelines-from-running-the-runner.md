---
id: ITEM-0100
title: Keep concurrent pipelines from running the runner out of memory
type: bug # feature | bug | debt | task
status: in-progress # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: []
depends_on: []
created: 2026-10-10
closed:
---

# ITEM-0100: Keep concurrent pipelines from running the runner out of memory

## Goal

Main's pipeline 795, on the merge of `plan-m08-m12`, failed when the
runner killed its unit-test job for memory (exit 137, a runner system
failure). The tag's pipeline, 796, ran on the same commit at the same
time, because the user tags a merge commit as soon as it lands
(ADR-0037). ITEM-0075 kept one pipeline's integration-test from sharing
memory with its unit-test, but not two pipelines' jobs. This keeps the
memory-heavy jobs to one at a time across the project's pipelines, so a
tag no longer fails main.

## Acceptance criteria

- [ ] `go-lint`, `unit-test`, `integration-test` and `release-check` share one GitLab resource group, so only one of them runs at a time across pipelines.
- [ ] `make project-lint` still passes: every job still runs only make targets, and none can skip.
- [ ] The next merge and its tag both pass, without a retry.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-10: Pipeline 795's unit-test log ends during `go test -race
  ./...`, about five minutes in, with "Error in container build: exit
  code: 137". The same job passed in the tag's pipeline. Measured locally
  with a cold build cache: `go test -race` of the whole repo peaks at
  1.28 GB in its largest process (1 min 40 s on 16 cores), and
  modernc.org/sqlite, which M08 adds (ADR-0038), at 0.95 GB, so the driver
  doesn't raise the peak. The two pipelines together did. GitHub's runners
  are a VM per job, so its workflow needs no change.
- 2026-10-10: Added the `.heavy` template, `resource_group:
  runner-memory`, to the four jobs. Within a pipeline they're already in
  different stages, so a pipeline on its own takes as long as before; a
  second pipeline's heavy jobs wait for the first's. Checking it needs the
  next merge and tag, at M08's close.
