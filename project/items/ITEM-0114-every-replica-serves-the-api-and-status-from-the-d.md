---
id: ITEM-0114
title: Every replica serves the API and status from the database
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: [REQ-033, REQ-052]
depends_on: [ITEM-0113]
created: 2026-10-18
closed:
---

# ITEM-0114: Every replica serves the API and status from the database

## Goal

Every replica serves from the database (ADR-0048): NetBox's zones, the
API's records, stored by the leader zone by zone, when a zone's digest
changes (migration `00007_netbox_zones.sql`); a refresh served once it's
committed, on either database; `Restore` made a reload, at start and
whenever the last refresh moves; `/status`'s deployment facts from the
database, and the replica's own facts its own, the spec saying which; the
drift gauges set on every replica; and a follower ready once it has
loaded the reports.

## Acceptance criteria

- [ ] A follower's `/api` answers, records included, match the leader's within the poll interval.
- [ ] After a restart, the API serves NetBox's records at once.
- [ ] A refresh that can't be written isn't served, on either database, and counts as a write failure.
- [ ] `/status`'s deployment facts are the same on every replica, and the spec says which facts are the replica's.
- [ ] A refresh that changes one zone writes that zone's row only.
- [ ] The docs on monitoring drift show how to aggregate the gauges across replicas, with a CHANGELOG line.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-18: Created from M09's approved design, at M09's start.
