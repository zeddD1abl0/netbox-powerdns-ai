---
id: ITEM-0122
title: Give golangci-lint enough time on CI's runner
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: []
depends_on: []
created: 2026-11-06
closed: 2026-11-06
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

- [x] golangci-lint's timeout leaves room for CI's 2-CPU runner with an empty cache, with the reason beside it.
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
- 2026-11-06: Pipeline 964 (dc80fc6, ITEM-0115) failed the same way, in
  2018 s. Job 5337 ran on `talos-control-temp`: `go vet` from 01:33:54 to
  01:43:49, 9 min 55 s, then golangci-lint from 01:43:54 until 02:06:45,
  22 min 51 s, when it failed with `Timeout exceeded`, and a goimports
  error from the cancelled run. So, on the runner before this item, with
  the job's 2Gi request: `go vet` took 10 min 24 s, 11 min 45 s and
  9 min 55 s (950, 953, 964), which is steady, and golangci-lint on the
  root module 2 min 37 s, 6 min 7 s and more than 22 min 51 s, which
  isn't. Locally, lint takes 0.27 of vet's time, as in 950 (0.25); 953
  took 0.52, and 964 more than 2.3. Only the step that needs the most
  memory slows, which points at the nodes' memory rather than their CPUs.
- 2026-11-06: Measured locally on two CPUs (`taskset -c 0,1`), each run
  with an empty `GOCACHE` and `GOLANGCI_LINT_CACHE` and a warm module
  cache, in a user scope whose anonymous memory was sampled every 100 ms.
  `make vet` took 173 s, at 1.15 GiB together; `make lint` after it 46 s,
  with golangci-lint at 1.96 GiB RSS (1.98 in a second run): more than the
  1.63 GiB measured on 16 CPUs (ITEM-0100), and right at the job's 2Gi
  request. `make lint` alone, from an empty build cache, took 198 s and
  peaked at 2.44 GiB. By `go vet`'s time, the runner's nodes are 3.5 to 4
  times slower than these two CPUs.
- 2026-11-06: Weighed:
  - Dropping `make vet` from the job, and so from `make check`: it would
    lose nothing, since golangci-lint's `govet` runs go vet's 35 analyzers
    but `loopclosure`, which reports nothing from Go 1.22 on, plus
    `inline` (its `GL_DEBUG=govet` log). But it saves little: golangci-lint
    then compiles what vet compiled, so lint alone takes 198 s against
    219 s for both, and peaks half a GiB higher, at 2.44 GiB, past the
    request. Kept: CI drops no check, so there's no decision to record.
  - Sharing the build cache: within the job, vet and golangci-lint already
    share one, and ADR-0032 keeps CI's jobs without a cache.
  - A longer timeout, or none. Done: none, `run.timeout: 0s`, with the
    reason beside it in `.golangci.yml`. In CI the job's timeout, an hour,
    bounds the run; 964's lint, at 22 min 51 s, already took more than a
    third of it, so a timeout of golangci-lint's own would have to be
    almost as long to leave room for that node.
  - Holding golangci-lint's memory, so that the job stays within its
    request on two CPUs. Done: `make lint` runs it with
    `GOMEMLIMIT=1536MiB`. After vet, on two CPUs, with no limit, it took
    46.3 s and peaked at 1.98 GiB; at 1536 MiB, 47.5 s and 1.53 GiB; at
    1280 MiB, 49.1 s and 1.28 GiB; at 1024 MiB, 53.5 s and 1.19 GiB, over
    its limit. So it keeps more than 1 GiB live, and 1536 MiB leaves room
    above that as the code grows, costs 3%, and leaves a quarter of the
    request for the job's other processes.
- 2026-11-06: After, with the Makefile's own targets, on the same two
  CPUs and empty caches: `make vet` 173 s, at 1.11 GiB together, then
  `make lint` 47.7 s, golangci-lint at 1.53 GiB. The `.heavy` comment in
  `.gitlab-ci.yml` has the two-CPU peak, and the CHANGELOG a line under
  Fixed. `make check` passes. The second criterion's "after" and the third
  criterion wait for the branch's next push: check that pipeline's
  `go-lint` job, its node, and its vet and lint times, and record them
  here and in ITEM-0100's and ITEM-0120's notes. If golangci-lint is still
  slow on `talos-control-temp` with the limit, the node, not the lint, is
  ITEM-0100's to settle.
