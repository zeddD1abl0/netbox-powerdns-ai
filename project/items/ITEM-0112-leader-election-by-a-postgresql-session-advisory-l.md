---
id: ITEM-0112
title: Leader election by a PostgreSQL session advisory lock
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: [REQ-033]
depends_on: [ITEM-0111]
created: 2026-10-18
closed:
---

# ITEM-0112: Leader election by a PostgreSQL session advisory lock

## Goal

The leader (ADR-0047): a session advisory lock on a key derived from the
database's ID, on a connection of its own, whose session sets TCP
keepalives, `tcp_user_timeout` and `idle_session_timeout`; a heartbeat
every 5 seconds, with a 5-second deadline, and a step-down at the first
miss; the fence on leader-only writes; refreshes and the prune on the
leader only; followers trying for the lock at each poll; the leader's row
(migration `00006_leader.sql`); `service.leader_acquired`; `/status`'s
`replica` and `leader`; and the three leader metrics. On SQLite, `serve`
always leads.

## Acceptance criteria

- [ ] With two `serve`s on one PostgreSQL database, one leads and refreshes, and the other follows, and takes over when the leader stops.
- [ ] A leader that misses a heartbeat steps down within 10 seconds, and a leader-only write after it has lost the lock fails the fence.
- [ ] The lock's session has the keepalive, user-timeout and idle-session settings, which a test checks.
- [ ] A takeover writes `service.leader_acquired`, with the previous leader in its before.
- [ ] `/status` and the metrics name the leader, and since when, on every replica; the spec, the contract tests and the generated references cover them.
- [ ] On SQLite, `serve` leads at once, as in M08, with no leader events.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-18: Created from M09's approved design, at M09's start.
