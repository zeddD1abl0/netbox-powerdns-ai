---
id: ITEM-0037
title: Test and support only PowerDNS 5.1, to fit the CI runners
type: task # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M02
requirements: [REQ-036, REQ-041]
depends_on: []
created: 2026-10-06
closed: 2026-10-06
---

# ITEM-0037: Test and support only PowerDNS 5.1, to fit the CI runners

## Goal

The first pipeline with M02's lab failed for lack of memory on the runners,
as M01's had (ITEM-0028). The user asked to concentrate on PowerDNS 5.1 for
the moment. Every supported release is tested, so support, the lab and the
tests narrow together to PowerDNS 5.1, with one server group in the lab.

## Acceptance criteria

- [x] ADR-0026 restates ADR-0024 with PowerDNS 5.1 as the only supported release, and supersedes it. REQ-041, Q-053's answer, the brief and M02's milestone file say so.
- [x] `powerdns.Supported` and the lab hold PowerDNS 5.1 only, and `TestSupportedMatchesLab` passes. The release check stays unit-tested against a list of two releases.
- [x] The lab runs no PowerDNS 5.0, and its recorded responses are gone from `testdata/`.
- [x] The docs, the generated references and the CHANGELOG say PowerDNS 5.1.
- [x] The emulated CI job's memory is measured and recorded here.
- [x] `make check` and `make test-integration` pass.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Created when the user reported that M02's first pipeline died
  for lack of memory, and asked to concentrate on PowerDNS 5.1. The user's
  words are in `project/brief.md`.
- 2026-10-06: Done.
  - **Decision:** ADR-0026 restates ADR-0024 with PowerDNS 5.1 as the only
    supported release, and supersedes it. REQ-041 keeps its ID, narrowed.
    Live code and docs cite ADR-0026; the brief, M02's approved design and
    the closed items keep ADR-0024, as history.
  - **Code and lab:** `powerdns.Supported` and `lab.PowerDNSes` hold 5.1
    only, so the lab has one server group, `lab-a`. `powerdns-50` and its
    recorded responses are gone. `TestServerReleases` keeps the check
    against two releases covered. The CLI's integration tests use `lab-a`;
    behavior across several groups stays covered by unit tests with local
    servers.
  - **PowerDNS now starts once NetBox is healthy** (`depends_on` in the
    compose file), so it's never running during NetBox's first-start
    migrations, when the lab's memory peaks.
  - **Measured** in the emulated CI job (ITEM-0028's method). "Resident" is
    the cgroup's `anon` memory, which is what an out-of-memory kill counts;
    "with cache" adds the page cache from pulling images.

    | Run | Resident: Docker-in-Docker and the lab | With cache | Job, resident |
    |---|---|---|---|
    | M01, which passed on the runners (ITEM-0028) | 1,119 MiB | 3,326 MiB | 323 MiB |
    | M02 with PowerDNS 5.1 and 5.0, which failed | 1,279 MiB | 4,131 MiB | 230 MiB |
    | PowerDNS 5.1 only | 1,163 MiB | 3,748 MiB | 336 MiB |
    | PowerDNS 5.1 only, started after NetBox | 1,240 MiB | 3,824 MiB | 313 MiB |

    The last two runs differ in NetBox, not PowerDNS: NetBox's own peak
    during its migrations was about 1.0 GiB in one and 1.11 GiB in the
    other, and in the last run PowerDNS wasn't yet running at the peak. So
    NetBox's peak varies by about 100 MiB from run to run, twice what a
    PowerDNS server uses (about 48 MiB). Dropping 5.0 and starting
    PowerDNS late take PowerDNS out of the peak, but the job's memory stays
    about where M01's was, and the runners' margin is within NetBox's
    variation. If the runners still evict it, the durable fixes are on the
    runner side (a memory request for the job, so it's only scheduled where
    it fits) or a lab NetBox that needs no first-start migrations.
  - The image left out saves about 0.25 GB of disk, so Docker-in-Docker
    stores about 2.2 GB. The GitLab job's comment and the lab how-to say so.
