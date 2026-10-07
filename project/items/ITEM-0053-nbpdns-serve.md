---
id: ITEM-0053
title: nbpdns serve
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M04
requirements: [REQ-003, REQ-014, REQ-044]
depends_on: [ITEM-0051, ITEM-0052]
created: 2026-10-07
closed:
---

# ITEM-0053: nbpdns serve

## Goal

`nbpdns serve`: refresh the drift report on a schedule, keep each group's
last-known state, answer `/livez` and `/readyz`, expose `/metrics` with
the drift metrics, and shut down cleanly (ADR-0029).

## Acceptance criteria

- [ ] `server.listen`, `drift.interval` (at least 10s) and `drift.timeout` load and validate.
- [ ] Refreshes start on schedule and never overlap, each one a trace with its own request ID; a failed group or NetBox keeps its last-known counts, with `up` at 0.
- [ ] `/readyz` and `/livez` follow ADR-0029, and SIGTERM stops the service cleanly, exiting 0.
- [ ] Unit tests and integration tests against the lab cover the schedule, the state, the endpoints and the drift metrics.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-07: Created from M04's approved design.
