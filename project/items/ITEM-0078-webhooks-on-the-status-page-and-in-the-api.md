---
id: ITEM-0078
title: Webhooks on the status page and in the API
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M07
requirements: [REQ-046, REQ-048]
depends_on: [ITEM-0077]
created: 2026-10-08
closed:
---

# ITEM-0078: Webhooks on the status page and in the API

## Goal

The `webhooks` section on `/status` and `/api/status`: whether webhooks
are on, the last event received, the pending zones, and the last zone
refresh. The API's version becomes 1.1.0.

## Acceptance criteria

- [ ] `/status`, in both forms, and `/api/status` show the section, documented and checked against the spec.
- [ ] The fields are added, never changed.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M07's approved design.
