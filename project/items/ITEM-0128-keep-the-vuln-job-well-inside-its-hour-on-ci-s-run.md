---
id: ITEM-0128
title: Keep the vuln job well inside its hour on CI's runner
type: bug # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: []
depends_on: []
created: 2026-11-25
closed:
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

- [ ] govulncheck's time and peak memory are measured on 2 CPUs with an empty cache, as ITEM-0100 measured the other jobs.
- [ ] The `vuln` job requests what it needs, or runs where it's measured to fit, so that it finishes well inside its hour on the runner.
- [ ] A branch pipeline shows it, and its node and duration are recorded here and in ITEM-0100.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-11-25: Found while recording pipeline 1040 for ITEM-0116. Both slow
  runs were on `talos-control-3`: in 1039, govulncheck alone took 22 min
  of the job's 1349 s. Pipeline 1041 (2e2da9f) hadn't reached its `vuln`
  job when this was written.
