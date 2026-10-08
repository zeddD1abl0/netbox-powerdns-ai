---
id: ITEM-0069
title: Zones and their changes in the API
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M06
requirements: [REQ-046]
depends_on: [ITEM-0068]
created: 2026-10-08
closed: 2026-10-08
---

# ITEM-0069: Zones and their changes in the API

## Goal

A group's zones, filtered by state and with unmanaged zones
included, one zone by name with or without its final dot, and the zone's
changed RRsets (ADR-0033).

## Acceptance criteria

- [x] `…/zones`, `…/zones/{zone}` and `…/zones/{zone}/changes` answer as the spec says, validated in table-driven tests.
- [x] `state` filters the zones, and a cursor reused with another filter is a 400.
- [x] An integration test on the lab finds a zone drifted on lab-a in `zones`, `changes` and `/api/status`, with every response validated.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M06's approved design.
- 2026-10-08: Done.
  - **The spec** gains `listZones`, `getZone` and `listZoneChanges`; the
    `ZoneName` path parameter, which can't be called `Zone`, since
    oapi-codegen would then generate two Go types named `Zone`; and the
    `ZoneState`, `Zone`, `ZoneDetail` (`Zone` and `as_of`, by `allOf`),
    `ChangeKind`, `Side` and `Change` schemas. The three page schemas now
    share `PageLinks`, `self` and `next`, by `allOf`, which removed
    vacuum's duplicate-description notes.
  - **`make generate`** now runs oapi-codegen before `gendocs`, since
    `gendocs` compiles the module, which fails while the generated
    interface is ahead of the handlers.
  - **Zones** are those NetBox assigns, with the primary's unmanaged ones
    as state `unmanaged`, with a null view, policy and serials, in
    canonical name order. A serial of 0, which a report gives for a side
    without one, is null.
    - `?state=` is checked against the enum, made canonical (sorted,
      without repeats), and carried in the cursor. The same states in
      another order are the same filter.
    - The cursor names the zone the page starts after, and the next page
      starts at the first zone ordered after it. So a refresh between
      pages skips no zone that's still there, even if the cursor's own
      zone went, which a test shows.
    - A group never read has an empty page, with a null `as_of`.
  - **A zone** is found by its name with or without the final dot, in any
    case, through `dns.ZoneName`. It's 404 for an unknown group, a group
    never read, or a zone the group doesn't have, each with its own
    detail.
  - **Changes** page in the report's canonical order, by a cursor of the
    last RRset's name and type, compared with `dns.CompareRRsets`. A zone
    in sync, ignored or unmanaged has none.
  - **Tests**, every response checked against the spec:
    - the canonical order, with unmanaged zones included, and the nulls;
    - five state filters, and a bad state;
    - pages at limit 3, filtered pages keeping their filter, a cursor
      refused with another filter or none, the same filter reordered, and
      a refresh between pages;
    - `getZone` with and without the dot, and in upper case, an unmanaged
      zone, and three 404s;
    - changes, at once and one page at a time, the empty cases, a 404 and
      a 400.

    `TestServe`, on the lab, finds the fixture's three drifted zones in
    `zones?state=drift,missing,inactive_in_netbox`, the drifted zone's
    five changes (three changed, one missing, one extra), and the zone in
    sync without its final dot.
  - **Docs:** "Service endpoints" and the CHANGELOG cover the zones and
    their changes.
