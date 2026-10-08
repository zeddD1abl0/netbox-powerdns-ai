---
id: ITEM-0070
title: DNS records from NetBox in the API
type: feature # feature | bug | debt | task
status: in-progress # open | in-progress | blocked | done | wontfix
milestone: M06
requirements: [REQ-046, REQ-047]
depends_on: [ITEM-0069]
created: 2026-10-08
closed:
---

# ITEM-0070: DNS records from NetBox in the API

## Goal

Each zone's RRsets as NetBox defines them, for IaC to read
(REQ-047). `serve` reads every active NetBox zone in the groups' views,
keeps them, and `…/zones/{zone}/rrsets` serves them, as of NetBox's last
successful read. `nbpdns drift` reads no more than before.

## Acceptance criteria

- [x] `drift.Options.ReadNetBox` reads every active zone in the groups' views, and `serve` sets it; a test shows `nbpdns drift` doesn't.
- [x] `…/rrsets` pages a zone's active RRsets, marks NetBox's managed records, carries `as_of`, and is 404 for a zone that isn't active in NetBox.
- [x] An integration test matches a zone's `rrsets` with NetBox's records.
- [ ] The scale check (REQ-043) records each refresh's time and the resident memory with NetBox's zones kept.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M06's approved design.
- 2026-10-08: Code done; the scale check is next.
  - **`drift.Options.ReadNetBox`** makes `Run` read the RRsets of every
    active NetBox zone in the groups' views, whatever the primaries do, and
    return them, sorted by view and name, as `Report.NetBox`. That field is
    `json:"-"`, so `nbpdns drift -o json` is unchanged.
    - Only the compared zones' problems reach `Compare`. Its rule keeps
      every NetBox problem in the group's views, so without the filter the
      extra reads would have added problems that `nbpdns drift` doesn't
      report.
    - `TestRunReadNetBox` shows the group reports identical with and
      without the option, an inactive zone and one in no group's view
      left unread, and a missing zone's problem left out.
  - **`serve` sets it** (`compare` now takes `drift.Options`). `TestDrift`
    shows, from the debug log, that `nbpdns drift` still reads exactly two
    zones' records, the two it compares, on the lab fixture.
  - **The service keeps NetBox's zones** from each refresh that read
    NetBox, indexed by view and name, as `NetBoxView`, with when they were
    read. It keeps the last ones when NetBox fails or a refresh times out.
    `NetBoxView.Zone` picks a zone from the first of a group's views, by
    name, that has it, as the comparison does.
  - **`…/zones/{zone}/rrsets`** pages the zone's RRsets with their active
    records, each `managed` or not, leaving out RRsets with none, in
    canonical order, with `as_of`, NetBox's last read. It's 404 before
    NetBox is read, and for a zone NetBox has no active zone of, in the
    group's views, with the views named. A zone missing on the primary has
    its records.
  - **Each zone gains `rrset_count`**, the number of RRsets it has with
    active records, or null.
  - **Tests:** active records only, managed records, a missing zone, the
    first view winning, paging, a 400, four 404s, and `rrset_count`.
    `TestServe`, on the lab, matches the zone in sync's and the missing
    zone's `rrsets` against `nbpdns netbox records`' active records,
    record for record.
