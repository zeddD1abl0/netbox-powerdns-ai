---
id: ITEM-0045
title: Drift fixture and integration tests
type: task # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M03
requirements: [REQ-036, REQ-043]
depends_on: [ITEM-0044]
created: 2026-10-06
closed: 2026-10-06
---

# ITEM-0045: Drift fixture and integration tests

## Goal

A drift fixture in the lab, with one of each case from ADR-0027 across the
lab's NetBox and lab-a's primary, and integration tests of `nbpdns drift` on
it. The lab gains no containers.

## Acceptance criteria

- [x] The fixture creates every case of the approved design in NetBox and on lab-a, and removes it afterwards.
- [x] Integration tests check each case, as a table and as JSON, with `--zone`, and with a group whose primary can't be read, and their exit codes.
- [x] `make test-integration` passes, and the lab's containers are unchanged.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Created from M03's approved design.
- 2026-10-06: Done. `internal/lab/drift_fixture.go` creates six zones in one
  NetBox view and on lab-a, one per case: in sync (with an SOA that differs
  only in its serial, and unserved records on both sides with different
  values), drift (a missing, an extra, a changed value, a changed TTL, and
  an SOA contact), missing on the primary, parked in NetBox and served,
  ignored by policy, and unmanaged. NetBox's SOA fields are set explicitly,
  so the fixture doesn't depend on the plugin's defaults. The record and
  PowerDNS zone creation the fixtures share became `adminClient.records`
  and `createPowerDNSZone`. `internal/cli/drift_integration_test.go`
  checks every case as JSON and as a table, `--zone` (reading only that
  zone on both sides, and failing for a zone that's nowhere), and a second
  group whose primary refuses connections (exit 1, lab-a still reported).
  Assertions on unmanaged zones only look for the fixture's own, since other
  packages' tests share lab-a. Breaking the serial exclusion makes the test
  fail, and the fixture leaves no zones behind. `make test-integration`
  passes, and `deploy/` is unchanged.
