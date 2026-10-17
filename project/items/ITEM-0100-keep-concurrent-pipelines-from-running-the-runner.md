---
id: ITEM-0100
title: Keep concurrent pipelines from running the runner out of memory
type: bug # feature | bug | debt | task
status: in-progress # open | in-progress | blocked | done | wontfix
milestone: M09
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
- 2026-10-17: Moved from M08 to M09, as the user chose at M08's close:
  the first two criteria hold since df0a4a5, but the last can only be
  checked on M08's merge and the v0.8.0 tag, and a done milestone can't
  hold an item in progress (`make project-lint`). It closes, on M09's
  branch, once those two pipelines have passed without a retry; if one
  fails for memory again, this item takes the fix.
- 2026-10-17: M08's merge and tag don't meet the last criterion. The
  merge request's pipeline, 819, and the tag's, 824, each passed only
  after a retry of `release-check`, whose first attempt was killed for
  memory (exit 137) while goreleaser built linux/amd64 and linux/arm64 at
  once; `main`'s, 822, passed with every job first time. The two kills, and
  795's, all ran on the Kubernetes node `talos-control-temp`; every heavy
  job on `talos-control-1` and `talos-control-3` passed, the retries
  included, and 822 and 824 didn't overlap. So the cause looks like that
  node's memory, not two pipelines at once, which the resource group
  covers. GitHub's runners passed every job first time. The fix is the
  user's to choose: keep CI's pods off that node, or give the heavy jobs
  memory requests (`KUBERNETES_MEMORY_REQUEST`, if the runner allows the
  overwrite) so that Kubernetes places them where they fit, and halve
  `release-check`'s peak with goreleaser's `--parallelism 1`.
- 2026-10-18: The user chose both fixes. They keep CI's pods off
  `talos-control-temp`, or give it more memory, in the cluster or the
  runner's configuration. The repository gives the heavy jobs memory
  requests, so that Kubernetes places them only where they fit, and runs
  goreleaser with `--parallelism 1` where CI builds the release. The item
  closes once a merge's and its tag's pipelines pass without a retry.
