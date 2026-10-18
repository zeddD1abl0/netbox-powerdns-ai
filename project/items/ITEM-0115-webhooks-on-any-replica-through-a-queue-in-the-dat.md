---
id: ITEM-0115
title: Webhooks on any replica, through a queue in the database
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: [REQ-033]
depends_on: [ITEM-0114]
created: 2026-10-18
closed:
---

# ITEM-0115: Webhooks on any replica, through a queue in the database

## Goal

Webhooks on any replica (ADR-0048, ADR-0036): a refresh queue in the
database (migration `00008_refresh_queue.sql`), which every replica, on
either database, writes a webhook's zones to; the leader gathers it, with
ADR-0036's delay and bounds, and deletes the rows its refresh covered once
that refresh is recorded; and `/status`'s webhooks section from the
database. This lifts M07's "a webhook reaches one replica".

## Acceptance criteria

- [ ] A webhook sent to a follower is refreshed by the leader, with ADR-0036's delay and bounds.
- [ ] A queued webhook survives a restart, and a takeover.
- [ ] `/status`'s webhooks section is the same on every replica.
- [ ] The docs on refreshing drift as NetBox changes say that any replica takes webhooks, with a CHANGELOG line.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-18: Created from M09's approved design, at M09's start.
