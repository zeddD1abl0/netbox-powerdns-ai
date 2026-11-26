---
id: ITEM-0128
title: Keep the vuln job well inside its hour on CI's runner
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: []
depends_on: []
created: 2026-11-25
closed: 2026-11-26
---

# ITEM-0128: Keep the vuln job well inside its hour on CI's runner

## Goal

CI's `vuln` job, `make vuln` (govulncheck), is getting slower on the
runner: 1349 s in pipeline 1039 and 3270 s in pipeline 1040, both on
`talos-control-3`, against the job's hour. Locally it takes minutes. It
doesn't extend `.heavy`, so it requests no memory and shares no resource
group, and Kubernetes can place it on a busy node, where it may be
starved. A job that crosses the hour fails the pipeline, and M09's merge
and tag pipelines must pass without a retry (ITEM-0100).

## Acceptance criteria

- [x] govulncheck's time and peak memory are measured on 2 CPUs with an empty cache, as ITEM-0100 measured the other jobs.
- [x] The `vuln` job requests what it needs, or runs where it's measured to fit, so that it finishes well inside its hour on the runner.
- [x] A branch pipeline shows it, and its node and duration are recorded here and in ITEM-0100.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-11-25: Found while recording pipeline 1040 for ITEM-0116. Both slow
  runs were on `talos-control-3`: in 1039, govulncheck alone took 22 min
  of the job's 1349 s. Pipeline 1041 (2e2da9f) hadn't reached its `vuln`
  job when this was written.
- 2026-11-26: Pipeline 1041 (2e2da9f) passed every job without a retry. Its
  `vuln` took 1376 s on `talos-control-3`, govulncheck on the root module
  22 min 5 s. Pipeline 1042 (d79b08f), the first with x/net v0.62.0, passed
  every job too: its `vuln` took 142 s on the same node, govulncheck 1 min
  36 s. The difference is govulncheck's two paths: it traces calls only into
  an imported package that has a known vulnerability. 1039 to 1041 imported
  two, in x/net's `html` package (GO-2026-6860 and GO-2026-6866), so they
  traced; since 5801a13 none has, and the scan only type-checks.
- 2026-11-26: Measured locally on two CPUs (`taskset -c 0,1`): `make vuln`
  from an empty `GOCACHE` and `GOMODCACHE`, as CI starts, in a user scope
  whose anonymous memory was sampled every 100 ms, with `/usr/bin/time -v`
  for the largest process. At HEAD, without tracing: 56 s, 2.38 GiB
  together, govulncheck 2.38 GiB. At 6f251c7, with the old x/net, tracing:
  111 s, 3.50 GiB together, govulncheck 3.49 GiB, on about 90% of one CPU.
  So the runner took 2.5 times as long as these two CPUs for the scan that
  doesn't trace, but 12 to 30 times as long for the one that does, which
  needs 1.1 GiB more. The job's pod requested no memory, on a node that it
  shares with the control plane: that points at the node's memory, as
  golangci-lint's slow runs did (ITEM-0122).
- 2026-11-26: govulncheck alone, tracing, from a warm cache, at each
  `GOMEMLIMIT`: none, 21 s and 3.47 GiB; 3072MiB, 21 s and 2.91 GiB;
  2560MiB, 28 s and 2.71 GiB; 2048MiB, 31 s and 2.65 GiB; 1536MiB, 37 s
  and 2.65 GiB. So it keeps about 2.6 GiB live, and no limit fits it in
  `.heavy`'s 2 GiB.
- 2026-11-26: The user says the runner's `memory_request_overwrite_max_allowed`
  is 3Gi, and `service_memory_request_overwrite_max_allowed` 2Gi, and that
  no other resource setting is applied, so a CPU request would be ignored.
  The runner fails a job that requests more than the maximum. Weighed:
  - Joining `.heavy`: its 2 GiB request is below the scan's live heap, and
    its resource group would hold the other pipelines' heavy jobs, as when a
    merge and its tag run together, while a scan that has taken 22 to 54
    minutes on the runner ran. Not done.
  - A request of its own, of at most 3Gi. Done: `vuln` requests 3Gi, and
    `make vuln` runs govulncheck with `GOMEMLIMIT=2560MiB`. 3072MiB costs
    nothing, but peaks at 2.91 GiB, almost all of the request; 2560MiB
    peaks at 2.71 GiB and costs a third of the scan's time; 2048MiB saves
    only 0.06 GiB more, and costs half.
  - Removing the runner's maximum, which the user offered: an empty maximum
    switches the overwrite off, so every job's request would be ignored.
    Kept at 3Gi.
- 2026-11-26: After, `make vuln` with the limit, from empty caches on the
  same two CPUs: tracing (6f251c7 with the new Makefile), 97 s and 85 s in
  two runs, at 2.80 and 2.74 GiB together, govulncheck 2.79 and 2.73 GiB;
  without (HEAD), 104 s, 2.53 GiB, govulncheck 2.52 GiB. The wall times are
  mostly the network: that run used 64 s of CPU. The `.heavy` comment in
  `.gitlab-ci.yml` now says that the runner fails a job that requests more
  than its maximum, where it said that the request took effect up to it.
  The CHANGELOG has a line under Fixed. The last two criteria wait for the
  branch's next push: check that its `vuln` job ran (a request above the
  maximum fails it before it starts), its node, any wait for memory, and
  its duration, and record them here and in ITEM-0100. That pipeline won't
  trace, since no imported package has a known vulnerability, so it shows
  the request and the job's placement, not how long a traced scan takes.
- 2026-11-26: Pipeline 1047 (d67ed82), the first with this fix, passed
  every job without a retry. Its `vuln` job, 5775, ran: the runner logged
  `"MemoryRequest" overwritten with "3Gi"`, so the request is within the
  maximum, and the pod was placed on `talos-control-2` after 4 s as
  `Pending`, with no `Insufficient memory` wait. The job took 72 s, and
  `make vuln` 64 s of it, against 142 s for 1042's on `talos-control-3`.
  The scan didn't trace, as expected, so a traced scan's time on the runner
  is still unmeasured; with the limit it took 85 to 97 s on two local CPUs.
  Done.
