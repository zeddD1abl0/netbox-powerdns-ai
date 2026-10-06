---
id: ITEM-0043
title: Compare zones and report drift
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M03
requirements: [REQ-024, REQ-031, REQ-043]
depends_on: [ITEM-0042]
created: 2026-10-06
closed:
---

# ITEM-0043: Compare zones and report drift

## Goal

Compare the zones NetBox assigns to a server group with what its primary
serves, in the shared model, and build the drift report (ADR-0027): zone
states, RRset changes, unmanaged and ignored zones, problems, and a failed
group's reason, with the two reading interfaces M03 defines.

## Acceptance criteria

- [ ] `internal/drift.Compare` is pure, and table tests cover every case in ADR-0027: missing, extra and changed RRsets, TTLs, the SOA without its serial, records neither side serves, missing, inactive, unmanaged and ignored zones, and problems.
- [ ] `internal/drift.Run` reads NetBox once for every group's views, then each primary, and marks a group that can't be read as failed without stopping the others.
- [ ] A benchmark compares 1,000 zones and 100,000 records on each side in under a second, and its time and allocations are recorded here.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Created from M03's approved design.
