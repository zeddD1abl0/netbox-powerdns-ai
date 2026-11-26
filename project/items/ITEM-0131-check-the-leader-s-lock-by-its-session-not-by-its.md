---
id: ITEM-0131
title: Check the leader's lock by its session, not by its PID alone
type: bug # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M10
requirements: [REQ-033]
depends_on: []
created: 2026-11-26
closed:
---

# ITEM-0131: Check the leader's lock by its session, not by its PID alone

## Goal

A leader-only write (`leaderTx`) checks, in its transaction, that the
session that leads still holds the leader's lock: ADR-0047's fence. It
looks for the lock in `pg_locks` under the session's backend PID, read when
the replica joined. Suppose PostgreSQL ends that session, and another
replica's new backend gets the same PID, then the lock. The check then
passes for the deposed leader, which could write a stale refresh, or prune,
before its heartbeat fails. A PID is reused only once the PID counter
wraps, so it's rare, but the fence is what stops a deposed leader from
writing.

## Acceptance criteria

- [ ] The check identifies the leader's session by more than its PID, such
  as its PID and `backend_start`, read when it joined, so a new session
  with the same PID fails it.
- [ ] A test fails the check for a session whose PID matches but whose
  start doesn't.
- [ ] "How nbpdns replicas choose a leader" says what the fence compares.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-11-26: Found by M09's `/code-review high`, on
  `main...m09-postgresql-and-ha`, and checked against the code: `holdsLock`
  in `internal/store/leader.go` matches `pid = $1` alone, with `ss.pid`
  from `Join`. Left for after M09, as the user chose.
