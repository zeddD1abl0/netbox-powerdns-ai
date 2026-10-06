---
id: ITEM-0028
title: Test and support only NetBox 4.7, to fit the CI runners
type: task # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M01
requirements: [REQ-036, REQ-040]
depends_on: []
created: 2026-10-06
closed: 2026-10-06
---

# ITEM-0028: Test and support only NetBox 4.7, to fit the CI runners

## Goal

The first pipeline with the integration job failed: the job needs more
memory than the runner nodes have to spare, because the lab runs a NetBox and
a PostgreSQL server for each supported release, 4.7 and 4.6. The user decided
to narrow to NetBox 4.7, and to add other releases, and their support, later.
Every supported release is tested, so support, the lab and the tests narrow
together.

## Acceptance criteria

- [x] ADR-0023 restates ADR-0020 with NetBox 4.7 as the only supported release, and supersedes it. REQ-040, Q-007's answer, the brief and M01's milestone file say so.
- [x] `netbox.Supported` and the lab each hold NetBox 4.7 with the plugin 1.7.x only, and `TestSupportedMatchesLab` still passes. The version check stays unit-tested against a list of two releases.
- [x] The lab runs no NetBox 4.6 or its PostgreSQL server, and the 4.6 responses are gone from `testdata/`.
- [x] The docs, the generated supported-versions reference and the CHANGELOG say NetBox 4.7.
- [x] The integration job's peak memory, before and after, is measured in an emulated CI job and recorded here, and the lab how-to gives the memory the lab needs.
- [x] `make check` and `make test-integration` pass.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Created when the user reported that the first pipeline died
  for lack of memory on the runners, and asked to narrow to NetBox 4.7. The
  user's words are in `project/brief.md`.
- 2026-10-06: Done.
  - **Decision:** ADR-0023 restates ADR-0020 with NetBox 4.7 and the plugin
    1.7.x as the only supported release, and supersedes it. It also weighs
    keeping 4.6 untested (rejected: support nothing tests) and a CI job per
    release (rejected for now: the pipeline's total demand doesn't fall).
    REQ-040 keeps its ID, narrowed, as REQ-026 did when its ADR was
    superseded. Code comments and the explanation page now cite ADR-0023;
    the brief, M01's approved design and ADR-0019 keep ADR-0020, as history.
  - **Code:** `netbox.Supported` and `lab.NetBoxes` hold 4.7 only. A new
    `TestStatusCheckReleases` keeps the check against several releases
    covered. The 4.6 responses in `internal/netbox/testdata/netbox-46/` are
    removed; re-record them with `-record` when a release is added back.
  - **Lab:** `netbox-46` and `postgres-46` are gone. The YAML anchors stay,
    so adding a release back is one service block and one PostgreSQL server.
  - **Measured** in an emulated CI job, as in ITEM-0022: the pinned
    `docker:dind` service and golang job image, a fresh Docker volume each
    run, `make test-integration`, and each container's cgroup sampled every
    second. "Resident" is cgroup `anon` memory; "with cache" is
    `memory.current`, which adds page cache from pulling images and the Go
    build cache.

    | | Before (4.7 and 4.6) | After (4.7) |
    |---|---|---|
    | Docker-in-Docker and the lab, resident | 2,125 MiB | 1,119 MiB |
    | Docker-in-Docker and the lab, with cache | 6,063 MiB | 3,326 MiB |
    | Job container, resident | 360 MiB | 323 MiB |
    | Job container, with cache | 1,054 MiB | 870 MiB |
    | Docker-in-Docker storage | 3.3 GB (ITEM-0022) | 2.0 GB |
    | Duration | 334 s | 299 s |

    Each NetBox peaks at about 1.04 to 1.12 GiB, while its first start
    applies the migrations; PostgreSQL peaks at 84 MiB and Valkey at 5 MiB.
    A third run, with hard limits of 1.5 GiB on the service and 768 MiB on
    the job, passed in 297 s: the page cache is reclaimed at the limit. The
    figures are in the GitLab job's comment and the lab how-to.
  - **For the user:** if the runners let a job set its own limits, the
    service's and job's memory requests can be set from those figures. The
    real pipelines haven't run this change yet; they run on the next push.
