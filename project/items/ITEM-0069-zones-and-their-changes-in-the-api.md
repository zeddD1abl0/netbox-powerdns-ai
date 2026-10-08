---
id: ITEM-0069
title: Zones and their changes in the API
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M06
requirements: [REQ-046]
depends_on: [ITEM-0068]
created: 2026-10-08
closed:
---

# ITEM-0069: Zones and their changes in the API

## Goal

A group's zones, filtered by state and with unmanaged zones
included, one zone by name with or without its final dot, and the zone's
changed RRsets (ADR-0033).

## Acceptance criteria

- [ ] `…/zones`, `…/zones/{zone}` and `…/zones/{zone}/changes` answer as the spec says, validated in table-driven tests.
- [ ] `state` filters the zones, and a cursor reused with another filter is a 400.
- [ ] An integration test on the lab finds a zone drifted on lab-a in `zones`, `changes` and `/api/status`, with every response validated.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M06's approved design.
