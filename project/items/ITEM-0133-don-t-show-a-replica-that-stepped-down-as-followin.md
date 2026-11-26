---
id: ITEM-0133
title: Don't show a replica that stepped down as following itself
type: bug # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M10
requirements: [REQ-033]
depends_on: []
created: 2026-11-26
closed:
---

# ITEM-0133: Don't show a replica that stepped down as following itself

## Goal

When a leader misses a heartbeat and steps down, it follows no replica.
But the leader's row still names it until another replica takes over, and
its next checks follow that row. So `/status` says that the replica is a
follower of itself, and `nbpdns_leader_since_timestamp_seconds` keeps its
own takeover. If its old session's lock lingers, as when it was cut off,
this lasts up to about 30 s, during an incident, when people read
`/status` most.

## Acceptance criteria

- [ ] A replica that doesn't lead never shows itself as the leader that it
  follows: `/status` and `/api/status` say that no replica is known to
  lead, until another one takes over.
- [ ] `nbpdns_leader_since_timestamp_seconds` agrees with them.
- [ ] A test steps a leader down with no other replica to take over, and
  checks what it shows.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-11-26: Found by M09's `/code-review high`, as a nit, on
  `main...m09-postgresql-and-ha`, and checked against the code:
  `leadUntil` in `internal/cli/leader.go` calls `Follow(nil)`, then `saw`
  and `try` follow the row that the next check reads. Left for after M09,
  as the user chose.
