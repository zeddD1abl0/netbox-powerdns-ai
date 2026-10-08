---
id: ITEM-0078
title: Webhooks on the status page and in the API
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M07
requirements: [REQ-046, REQ-048]
depends_on: [ITEM-0077]
created: 2026-10-08
closed: 2026-10-08
---

# ITEM-0078: Webhooks on the status page and in the API

## Goal

The `webhooks` section on `/status` and `/api/status`: whether webhooks
are on, the last event received, the pending zones, and the last zone
refresh. The API's version becomes 1.1.0.

## Acceptance criteria

- [x] `/status`, in both forms, and `/api/status` show the section, documented and checked against the spec.
- [x] The fields are added, never changed.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M07's approved design.
- 2026-10-08: `/status`'s JSON gains `webhooks`: `enabled`,
  `delay_seconds`, `last_event` (when, the event, the object type, and
  NetBox's request ID and user), `pending` (its events, zones, whether
  full, and when it's due), and `last_refresh` (when, its zones or full
  with a reason, outcome, error, events, and NetBox's requests). The text
  page has the same, under "NetBox's webhooks". A scheduled refresh that
  covers queued zones is kept as full, "the scheduled refresh came first".
  `/api/status` maps it onto new schemas, with null for nothing, as the
  rest of the API does, and the API's status test covers each shape
  against the spec. "Service endpoints" documents every field, as its test
  checks.
