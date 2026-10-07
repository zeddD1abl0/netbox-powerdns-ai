---
id: ITEM-0055
title: The status page
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M04
requirements: [REQ-014]
depends_on: [ITEM-0053]
created: 2026-10-07
closed:
---

# ITEM-0055: The status page

## Goal

`/status` shows the service's state: the schedule, the refreshes, NetBox
and each group, with their drifted zones, and the trace export. It's
human-readable text by default, and JSON with `?json=1` (ADR-0029).

## Acceptance criteria

- [ ] `/status` gives the state ADR-0029 and the plan list, as text by default and as JSON with `?json=1`, before and after the first refresh.
- [ ] Every JSON field is in the reference page "HTTP endpoints", which a test checks, and neither form holds a token, key or header.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-07: Created from M04's approved design.
