---
id: ITEM-0122
title: Give golangci-lint enough time on CI's runner
type: bug # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: []
depends_on: []
created: 2026-11-06
closed:
---

# ITEM-0122: Give golangci-lint enough time on CI's runner

## Goal

Pipeline 953 (26f2217) failed in its `go-lint` job, before any test ran:
golangci-lint reported `0 issues`, then `Timeout exceeded: try increasing
it by passing --timeout option`, as it ran past `.golangci.yml`'s
`run.timeout` of 5 minutes. The runner's nodes have 2 CPUs, and each job
starts with an empty build cache, so the job has grown with the code: 632 s
in pipeline 836, 691 s in 845, 949 s in 950, and 1115 s in 953, of which
`go vet` took about 12 minutes and golangci-lint over 6. In 950,
golangci-lint took 2 min 37 s. A lint that times out on clean code fails
every pipeline after it, and skips the tests, so the limit should leave room
for the runner, and the job should spend less time compiling twice.

## Acceptance criteria

- [ ] golangci-lint's timeout leaves room for CI's 2-CPU runner with an empty cache, with the reason beside it.
- [ ] The `go-lint` job's time is measured on the runner before and after, and recorded here.
- [ ] The branch's next pipeline passes its `go-lint` job.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-11-06: Found while checking pipeline 953 for ITEM-0120, at the start
  of ITEM-0115's session. Job 5287 ran on `talos-control-1`: `go vet`, from
  03:16:06 to 03:27:51, then golangci-lint from 03:27:55 to 03:34:01, which
  stopped it. Options to weigh: a longer `run.timeout`, or none, since the
  job's own timeout of an hour bounds it; and sharing the build cache
  between `go vet` and golangci-lint, which compile the same packages, or
  dropping `make vet` from the job, since golangci-lint runs `govet` too.
