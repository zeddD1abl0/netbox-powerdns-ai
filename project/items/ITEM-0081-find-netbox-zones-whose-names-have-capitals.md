---
id: ITEM-0081
title: Find NetBox zones whose names have capitals
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M07
requirements: [REQ-048]
depends_on: []
created: 2026-10-08
closed: 2026-10-08
---

# ITEM-0081: Find NetBox zones whose names have capitals

## Goal

The DNS plugin keeps the case that a zone's name is given in, and its
`name` filter matches exactly: `Case.Example` isn't found by
`name=case.example`, as the lab's NetBox 4.7 showed on 2026-10-08.
nbpdns lowercases names, so `nbpdns drift --zone` and `nbpdns netbox
records --zone` couldn't find a zone with capitals in its name. M07's zone
refreshes list zones by name too, and would have taken such a zone for
deleted. The client now filters with `name__ie`, which matches whatever
the case, and takes several names at once.

## Acceptance criteria

- [x] `netbox.ZoneFilter` takes `Names`, sent as `name__ie`, and a unit test checks every filter's query.
- [x] The NetBox integration test finds the fixture's zone by its name in capitals.
- [x] The CHANGELOG has the fix.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Found while designing ITEM-0077's listing. Several
  `name__ie` values are ORed, as several `name` values are, so a zone
  refresh lists all its zones in one request.
