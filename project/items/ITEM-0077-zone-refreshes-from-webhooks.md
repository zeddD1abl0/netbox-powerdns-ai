---
id: ITEM-0077
title: Zone refreshes from webhooks
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M07
requirements: [REQ-044, REQ-048]
depends_on: [ITEM-0076]
created: 2026-10-08
closed:
---

# ITEM-0077: Zone refreshes from webhooks

## Goal

Zone refreshes (ADR-0035): `drift.Options.Zones` compares a set of
zones; the service queues the zones that webhooks name, waits for
`drift.webhook_delay` of quiet, or 30 s, and compares only those zones,
or makes a full refresh past 100 zones or for a view; the results merge
into the kept state, metrics and NetBox's records; and each zone refresh
is a trace linked to its webhooks, with NetBox's request IDs and users.

## Acceptance criteria

- [ ] A burst makes one zone refresh after the quiet spell, or a full one past 100 zones or for a view, and zone and scheduled refreshes never overlap, as tests show.
- [ ] A zone refresh replaces only those zones' entries, counts, metrics and records; a zone gone from both sides leaves; a failing group keeps its report.
- [ ] The zone refresh's trace links to the webhook spans, and carries NetBox's request IDs and users, as its logs do.
- [ ] `drift.webhook_delay` and the zone-refresh metrics are declared, and the references regenerated.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M07's approved design.
