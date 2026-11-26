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

- [x] `go-lint`, `unit-test`, `integration-test` and `release-check` share one GitLab resource group, so only one of them runs at a time across pipelines.
- [x] `make project-lint` still passes: every job still runs only make targets, and none can skip.
- [x] Those jobs, and `release`, request memory from the Kubernetes runner, sized from their measured peaks: 2 GiB for each job, and 1.5 GiB for the integration lab's Docker-in-Docker service.
- [x] GoReleaser builds one architecture at a time (`--parallelism 1`) in both `make release-check` and `make release`.
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
- 2026-10-18: Measured on 16 CPUs, each with an empty build cache, with
  `/usr/bin/time -v` for the largest process and the cgroup's anonymous
  memory, sampled every 100 ms, for the job's processes together. A pod is
  killed on the total. Together, and largest: GoReleaser's snapshot at its
  default parallelism, 1.25 and 0.68 GiB, in 46 s; with `--parallelism 1`,
  0.82 and 0.80 GiB, in 40 s; `make test`, 1.62 and 1.30 GiB, in 2 min 46 s;
  `make lint`, 1.60 and 1.63 GiB (golangci-lint itself), in 57 s; and the
  integration tests' `-race` build, without running them, 1.65 and
  1.41 GiB, in 1 min 38 s. The lab's service adds 1.1 to 1.25 GiB
  (ITEM-0028, ITEM-0037). On a warm cache the snapshot takes 11.5 s at the
  default, and 13.6 s with `--parallelism 1`.
- 2026-10-18: The runner is GitLab's Helm chart, deployed by OpenTofu, and
  its `runners.config` sets no resources: no requests, limits or overwrite
  maxima, so every CI pod is BestEffort, the first that the kernel kills
  when a node runs short. The nodes have 2 CPUs each, and 8 GB
  (`talos-control-1`, `-3`) or 6 GB (`talos-control-temp`), so they compile
  less at once than the measurements above, which bound them. Done:
  `--parallelism 1` in both GoReleaser targets, through the Makefile's
  `GORELEASER_RUN`, with no variable for CI to set, since it costs about
  2 s; `.heavy` requests 2 GiB (`KUBERNETES_MEMORY_REQUEST`), the largest
  peak plus about a fifth; the integration lab's service requests 1.5 GiB
  (`KUBERNETES_SERVICE_MEMORY_REQUEST`); `release` joins `.heavy`; and no
  limits, since the runner sets none and the peaks grow with a node's CPUs.
  The requests take effect only once the user adds
  `memory_request_overwrite_max_allowed` and
  `service_memory_request_overwrite_max_allowed` to the runner's config;
  until then GitLab ignores them, silently. The last criterion waits for a
  merge and its tag.
- 2026-10-22: The M09 branch's pipelines so far, 832 (821cb9e, this
  item's fix) and 836 (ec27fff), passed with every job first time:
  836's integration job took 23 min, and `release-check` 7 min. ITEM-0110's
  commit, 951cb7c, isn't pushed yet, so its pipeline, the first to run the
  store's tests on PostgreSQL, is still to come. The item still waits for a
  merge and its tag.
- 2026-11-04: Pipeline 845 (f8e728a), the first to run the store's tests
  on PostgreSQL, ran every job without a retry. Its integration job got
  its 2Gi request and its services their 1536 MiB, and ran to the end in
  1406 s without an OOM kill, but failed two tests on a long host name
  (ITEM-0120), so it doesn't count as a pass. The item still waits for a
  merge and its tag.
- 2026-11-05: Pipeline 950 (9573c69) ran every job without a retry. Its
  integration job ran to the end in 1584 s, without an OOM kill, but
  failed five tests on the runner's long host name (ITEM-0120), so it
  doesn't count as a pass. The item still waits for a merge and its tag.
- 2026-11-06: Pipeline 953 (26f2217, ITEM-0120's fix) failed in its
  `go-lint` job, without an OOM kill or a retry: golangci-lint ran past
  `.golangci.yml`'s 5-minute timeout on the 2-CPU node `talos-control-1`,
  after `go vet` took about 12 minutes (ITEM-0122). The later stages were
  skipped, so it doesn't count as a pass. The item still waits for a merge
  and its tag.
- 2026-11-06: Pipeline 964 (dc80fc6, ITEM-0115) failed in its `go-lint`
  job, without an OOM kill or a retry. Job 5337 got its 2Gi request and
  ran on `talos-control-temp`, the node that the note of 2026-10-18 keeps
  CI's pods off, or gives more memory, so CI's pods still reach it. There
  `go vet` took 9 min 55 s, about as long as on `talos-control-1`, but
  golangci-lint ran for 22 min 51 s, past its 5-minute timeout, and then
  failed (ITEM-0122). The later stages were skipped, so it doesn't count
  as a pass. The item still waits for a merge and its tag.
- 2026-11-25: Pipeline 965 (720db36, ITEM-0122) ran every job that it
  reached without an OOM kill. Its first `go-lint` job, 5348, ran on
  `talos-control-temp` on 2026-11-06 and ran into the job's hour in
  golangci-lint (ITEM-0122). The user added a node with more memory,
  `talos-control-2`, and retried the job on 2026-11-25: it passed there in
  6 min, and `unit-test` (398 s) and `integration-test` (1047 s) ran there
  too. The integration pod waited about 40 s at first, as `0/3 nodes are
  available: 3 Insufficient memory`, until its requests fitted; it then ran
  to the end, but failed one test on its own expectation (ITEM-0124), so
  it doesn't count as a pass. Later stages were skipped. The item still
  waits for a merge and its tag.
- 2026-11-25: Pipeline 1039 (e1b7805) passed every job, without an OOM
  kill or a retry, the first branch pipeline to reach its last stage
  since M09 started. No job ran on `talos-control-temp`. `go-lint` (359
  s), `unit-test` (400 s), `integration-test` (1028 s), `release-check`
  (303 s) and `docs-site` ran on `talos-control-2`; `vuln` (1362 s) and
  `secrets` on `talos-control-3`, where govulncheck alone took 22 min.
  The `integration-test` and `release-check` pods each waited about 30 s
  as `0/3 nodes are available: 3 Insufficient memory` before they were
  scheduled. A branch pipeline doesn't count: the item still waits for
  M09's merge and its tag.
- 2026-11-25: Pipeline 1040 (7d5f4d6) passed every job, without an OOM
  kill or a retry: `go-lint`, `unit-test`, `integration-test` (1089 s)
  and `release-check` on `talos-control-2`, and `build`, `vuln` and
  `secrets` on `talos-control-3`. `vuln` took 3270 s there, more than
  twice 1039's 1349 s, and close to the job's hour: ITEM-0128. Pipeline
  1041 (2e2da9f) had passed its lint and test stages, on the same nodes,
  when this was written. Branch pipelines don't count: the item still
  waits for M09's merge and its tag.
- 2026-11-26: Pipelines 1041 (2e2da9f) and 1042 (d79b08f) passed every
  job, without an OOM kill or a retry. In both, `go-lint` and `unit-test`
  ran on `talos-control-2`, and `integration-test` (1101 s and 1090 s) and
  `release-check` on its CPU, the i5-6260U (their logs name the CPU, not
  the node), each after its pod waited about 30 s as `Insufficient
  memory`. `build`, `vuln` and `secrets` ran on `talos-control-3`. `vuln`
  took 1376 s in 1041, and 142 s in 1042, the first after x/net v0.62.0,
  since its scan no longer traces calls into a vulnerable package
  (ITEM-0128). The user has removed `talos-control-temp` from the cluster,
  so no job can run there again. Branch pipelines don't count: the item
  still waits for M09's merge and its tag.
- 2026-11-26: Pipeline 1047 (d67ed82, ITEM-0128's fix) passed every job,
  without an OOM kill or a retry. `go-lint` (342 s), `unit-test` (401 s)
  and `vuln` (72 s) ran on `talos-control-2`; `vuln` now requests 3Gi, which
  the runner applied, and its pod didn't wait for memory. `integration-test`
  (1099 s) and `release-check` (263 s) ran on its CPU, the i5-6260U, after
  their pods waited 28 s and 31 s as `Insufficient memory`. The other lint
  jobs, `build`, `docs-site` and `secrets` ran on `talos-control-3`, or on
  its QEMU CPU. Branch pipelines don't count: the item still waits for
  M09's merge and its tag.
