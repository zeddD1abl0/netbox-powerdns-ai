---
id: ITEM-0043
title: Compare zones and report drift
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M03
requirements: [REQ-024, REQ-031, REQ-043]
depends_on: [ITEM-0042]
created: 2026-10-06
closed: 2026-10-06
---

# ITEM-0043: Compare zones and report drift

## Goal

Compare the zones NetBox assigns to a server group with what its primary
serves, in the shared model, and build the drift report (ADR-0027): zone
states, RRset changes, unmanaged and ignored zones, problems, and a failed
group's reason, with the two reading interfaces M03 defines.

## Acceptance criteria

- [x] `internal/drift.Compare` is pure, and table tests cover every case in ADR-0027: missing, extra and changed RRsets, TTLs, the SOA without its serial, records neither side serves, missing, inactive, unmanaged and ignored zones, and problems.
- [x] `internal/drift.Run` reads NetBox once for every group's views, then each primary, and marks a group that can't be read as failed without stopping the others.
- [x] A benchmark compares 1,000 zones and 100,000 records on each side in under a second, and its time and allocations are recorded here.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Created from M03's approved design.
- 2026-10-06: Done.
  - **`internal/drift.Compare`** is pure. For each NetBox zone of the group's
    views: `ignored` by policy; `inactive_in_netbox` if it isn't active and
    the primary serves it (and not listed if it isn't active and absent, as
    expected); `missing` if the primary doesn't have it; otherwise its
    served RRsets are compared, `in_sync` or `drift`. Zones only on the
    primary are `unmanaged`. RRsets are keyed by name and type over active
    records only, with each side's RRset TTL; an SOA is compared without its
    serial, and both serials are in the zone's report. Changes are sorted
    as the model sorts RRsets (`dns.CompareRRsets`, now exported).
  - **Warnings**, not errors: a zone name in two of a group's views (the
    first view's, in name order, is compared), and a `zone_policies` entry
    for a zone the group doesn't serve.
  - **`internal/drift.Run`** reads NetBox once, for the union of every
    group's views, and reads RRsets only for zones some group compares, once
    each; then each primary, reading RRsets only for the zones its group
    compares. If NetBox can't be read, Run returns the error; a group whose
    client couldn't be built, or whose primary fails to list or read, is
    `failed`, and the report isn't `complete`. Its two interfaces,
    `drift.NetBox` and `drift.Primary`, are the backend interfaces Q-039
    left to M03.
  - **Tests:** a table test per rule of ADR-0027; a group-level test of
    unmanaged zones, shared names, problems kept and dropped, warnings and
    counts; and `Run` through in-memory sources, which implement M03's own
    interfaces, not NetBox's or PowerDNS's APIs: what's read, one zone, and
    NetBox and a primary failing.
  - **Scale (REQ-043):** `BenchmarkCompareAtScale` compares 1,000 zones of
    100 records on each side (100,000 records, 990 changed): about 104 ms
    and 124 MB allocated per comparison (1.83 million allocations), on a
    16-thread workstation, against the one-second bound.
    `TestScaleZones` checks the generated sizes.
