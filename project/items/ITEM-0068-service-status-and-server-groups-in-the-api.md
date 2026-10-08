---
id: ITEM-0068
title: Service status and server groups in the API
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M06
requirements: [REQ-046]
depends_on: [ITEM-0067]
created: 2026-10-08
closed:
---

# ITEM-0068: Service status and server groups in the API

## Goal

`/api/status`, `/api/server-groups` and `/api/server-groups/{group}`,
read from a snapshot of the service's state, with the cursor pagination
every list uses (ADR-0033).

## Acceptance criteria

- [ ] `Service.Snapshot` copies the state under its lock, and the API reads it through an interface.
- [ ] `/api/status` and both group resources answer as the spec says, validated in table-driven tests.
- [ ] Lists page with `limit` and `cursor`, with `self` and `next`, and refuse a bad limit or cursor with a 400.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M06's approved design.
- 2026-10-08: `/api/status` was built in ITEM-0066, as the pipeline's first operation. This item keeps the server groups and the pagination.
