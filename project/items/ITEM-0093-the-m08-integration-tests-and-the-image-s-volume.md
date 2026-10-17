---
id: ITEM-0093
title: The M08 integration tests and the image's volume
type: task # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-036, REQ-049, REQ-050, REQ-052]
depends_on: [ITEM-0089, ITEM-0091, ITEM-0092]
created: 2026-10-09
closed: 2026-10-15
---

# ITEM-0093: The M08 integration tests and the image's volume

## Goal

The integration tests on the lab: a restart that keeps the reports and
history; a second `serve` refused; settings and groups changed with the CLI
reaching a running `serve`; `nbpdns audit verify` passing, then failing
after an edit. And the image: the release check runs it with a volume owned
by 65532 at `/var/lib/nbpdns`, and writes the database.

## Acceptance criteria

- [x] The integration tests pass in CI.
- [x] The release check writes the database in the image, on a volume.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-09: Created from M08's approved design, on `plan-m08-m12`.
- 2026-10-15: The plan, agreed with the user: the integration tests in
  `internal/cli/serve_integration_test.go`, on the drift fixture, reusing
  `startServe`, `run`, `waitFor` and `keyFile`, and `labGroup`, which
  `TestServe`'s config closure became.
  - `TestServeRestarts`: a second `serve` refused while the first runs;
    the restart has NetBox unreachable, so that what it serves at once can
    only be what it kept, which the user agreed tests "at once": `/readyz`
    200 before any refresh, `/api/status`'s `database.restored` and
    `restored_as_of`, the same drifted zones and history in the API and
    `nbpdns drift history`. Then `nbpdns audit verify` passes on the lab's
    trail, and fails at the drifted zone's `zone.drift_changed` once it's
    edited to say `in_sync`.
  - `TestServeAppliesCLIChanges`: `nbpdns settings set drift.interval 15s`
    moves the next refresh from 5m to 15s after the last one's start;
    `nbpdns groups add lab-cli`, on lab-a's primary and view, with its
    check passing against the lab's PowerDNS, is compared within seconds,
    managed by `cli`, with lab-a's counts, and audited with `check:
    passed`.
- 2026-10-15: The release check's subtest "serve without groups" was
  stale since ITEM-0091, when `serve` started without groups, so `make
  release-check` failed on the branch; "serve without a volume" replaces
  it, and expects the missing `/var/lib/nbpdns`, writable by user 65532.
  "serve on a volume" runs `serve` with no groups, a read-only root and no
  network on two mounts of a directory owned by 65532: a named volume,
  and, as the user asked on 2026-10-15, a bind mount too. CI's release job
  talks to Docker-in-Docker over tcp, which can't bind-mount the job's
  directories, so both directories are in one named volume, made by a tar
  archive that the API extracts through a container that never starts:
  the API sets the owner of the directories it makes, but not of the one
  it extracts into, so a volume's own root can't become 65532's that way.
  The named volume mounts its directory as a subpath (API 1.45, Docker
  26; CI's DinD is 29.8.1), and the bind mount by its path on Docker's
  host, the volume's mount point. Each run sees a second `serve` refused
  on the lock across containers, `serve` stop with 0 having opened
  `/var/lib/nbpdns/nbpdns.db`, and a new container read its start and
  stop with `nbpdns audit list`. With the directories owned by root, the
  subtest fails with "permission denied". The deployment docs keep the
  bind mount owned by 65532 (ADR-0038).
- 2026-10-15: Found ITEM-0106: right after the restart, `/api/status`
  failed its contract with a negative `duration_seconds`. Fixed in this
  session, since it blocked `TestServeRestarts`. Created ITEM-0105 from
  ITEM-0104's question, which the user answered.
- 2026-10-15: Done. `make check` passed; `make test-integration` passed
  (internal/cli 89.7 s, internal/netbox 6.7 s, internal/powerdns 1.2 s),
  run locally, as CI's integration job runs it; the pipeline confirms it
  on the push. `make release-check` passed, with both mounts. No CHANGELOG
  line or docs: only tests changed, and ITEM-0094 owns M08's docs.
