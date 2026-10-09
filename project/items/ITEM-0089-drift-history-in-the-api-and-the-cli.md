---
id: ITEM-0089
title: Drift history in the API and the CLI
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-046, REQ-052]
depends_on: [ITEM-0088]
created: 2026-10-09
closed:
---

# ITEM-0089: Drift history in the API and the CLI

## Goal

The history for readers: `GET /api/server-groups/{group}/zones/{zone}/history`
and `GET /api/server-groups/{group}/refreshes`, paged with cursors, in the
spec first; `nbpdns drift history`; and the API's version 1.2.0.

## Acceptance criteria

- [ ] The operations answer as the spec says, each response checked against it.
- [ ] `nbpdns drift history` shows a zone's history as text and JSON.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-09: Created from M08's approved design, on `plan-m08-m12`.
