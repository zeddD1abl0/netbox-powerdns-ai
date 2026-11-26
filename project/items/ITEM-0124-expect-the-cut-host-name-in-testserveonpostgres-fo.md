---
id: ITEM-0124
title: Expect the cut host name in TestServeOnPostgres for a PID of any length
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: [REQ-033]
depends_on: []
created: 2026-11-25
closed: 2026-11-25
---

# ITEM-0124: Expect the cut host name in TestServeOnPostgres for a PID of any length

## Goal

Pipeline 965 (720db36) failed its integration job on one test,
`TestServeOnPostgres`, in `internal/cli`. ITEM-0120 cuts a long host name
short in its middle, so that a lock holder's `application_name` keeps its
PID within PostgreSQL's 63 bytes, and the test expected the host's first 20
characters before the `...`. But how much of the host is kept depends on
the PID's length: on the runner's 49-character host name, a 4-digit PID
keeps 19 characters, a 6-digit one 18, and only a 2-digit one 20. The test
passed on a short host name, as on a developer's machine, and failed on the
runner's. The code is right; the test should check the name's form, not a
length that the PID decides, since the store's `TestApplicationName`
already checks the exact cut.

## Acceptance criteria

- [x] `TestServeOnPostgres` fails as pipeline 965's did on the runner's host name, before the fix (reproduced locally).
- [x] It passes on the runner's host name with PIDs of different lengths, and on a short host name.
- [x] `make test-integration` passes, under the runner's host name as well.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-11-25: Found in pipeline 965 (720db36), the first pipeline whose
  integration job ran ITEM-0120's fix on the runner. Job 5353 ran on
  `talos-control-2`, the cluster's new node, without an OOM kill or a
  retry; only this test failed: the error named `host
  runner-d7rzzl1zg-pr...current-0-1lxgs2w8, PID 7970`, and the test wanted
  `host runner-d7rzzl1zg-pro`.
- 2026-11-25: Done. The test now finds each holder in the error with
  `host (\S+), PID <its PID>, since `, wants two, and checks each name with
  `namesHost`: the host name whole, or its two ends around `...`, shorter
  than it. Reproduced and checked locally in a user namespace with the
  runner's host name (`unshare -r --uts`, then `hostname`): before the
  fix it failed with a 6-digit PID, which keeps 18 characters; after it,
  it passes with 6-digit PIDs, with small ones in a PID namespace
  (`--pid --fork --mount-proc`), and on the machine's own short host name.
  The five integration packages pass under the runner's host name. `make
  test-integration` failed once on `TestServeRetriesAFailedReload`
  (ITEM-0123's flaky test; its message wasn't kept), and passed when run
  again. `make check` passes, but for `vuln`, which now finds GO-2026-6859 in Go 1.27.3,
  ITEM-0125's. No CHANGELOG line or docs: a test's fix.
