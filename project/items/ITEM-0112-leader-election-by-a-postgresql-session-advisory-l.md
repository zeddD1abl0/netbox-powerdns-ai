---
id: ITEM-0112
title: Leader election by a PostgreSQL session advisory lock
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: [REQ-033]
depends_on: [ITEM-0111]
created: 2026-10-18
closed: 2026-10-22
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

- [x] With two `serve`s on one PostgreSQL database, one leads and refreshes, and the other follows, and takes over when the leader stops.
- [x] A leader that misses a heartbeat steps down within 10 seconds, and a leader-only write after it has lost the lock fails the fence.
- [x] The lock's session has the keepalive, user-timeout and idle-session settings, which a test checks.
- [x] A takeover writes `service.leader_acquired`, with the previous leader in its before.
- [x] `/status` and the metrics name the leader, and since when, on every replica; the spec, the contract tests and the generated references cover them.
- [x] On SQLite, `serve` leads at once, as in M08, with no leader events.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-18: Created from M09's approved design, at M09's start.
- 2026-10-22: Built. Decisions settled before coding, as the session's
  prompt asked:
  - **db migrate and rotate-master-key with several replicas:** every
    replica holds the serve lock (`serveLockKey`) *shared*, through
    `Store.Join`, on its own session; `Store.Lock`, which `db migrate` and
    `secrets rotate-master-key` take, stays *exclusive*. So they refuse
    while any replica runs, followers too (followers hold the old master
    key), and a replica refuses to start while one of them runs. The
    `LockedError` names up to three holders, and says "N other nbpdns
    processes hold" for several. The leader's lock is a second key, 56 bits
    of a SHA-256 of the database's ID, so that two installs in two schemas
    of one database never share it.
  - **Follower poll:** `leaderPoll`, 5 seconds, a package variable in
    `internal/cli/leader.go`, until ITEM-0113's `database.poll_interval`
    replaces it. Tests shorten it. The heartbeat's interval and deadline
    are 5 seconds each, also variables.
- 2026-10-22: How it fits together: `internal/store/leader.go` has
  `Join` (the replica's session: settings, application_name, the shared
  serve lock), `Session.TryLead` (the leader's lock, then the leader's row
  and `service.leader_acquired` in one fenced transaction), `Heartbeat`
  (an update of the row on the lock's session), `Close`, `Store.Leader`,
  and `leaderTx`, the fence: once a store has joined, `RecordRefresh`,
  `PruneDriftHistory` and `PruneAudit` check `pg_locks` for the leading
  session's PID after the write lock, and fail with `ErrNotLeader`. A
  store that never joined (the CLI's, SQLite's) checks nothing.
  `internal/cli/leader.go`'s `election` runs the loop; serve's leader work
  (`svc.Run` and the prune) runs only inside it on PostgreSQL. On a missed
  heartbeat it cancels the work, ends the session, waits for the work, then
  follows; on stop, it stops the work first, then ends the session.
  Migration `00006_leader.sql` holds the row.
- 2026-10-22: Liveness: a follower never refreshes, so `/livez` would have
  failed it; now a follower is live while its election polls (within a
  minute), and the leader's refresh clock counts from its takeover.
  `nbpdns_leader_changes_total` counts the changes that a replica saw (its
  own takeovers from another replica, and new leaders that it read as a
  follower), so `max` across replicas gives the deployment's count.
- 2026-10-22: Left for later items, and said in the PostgreSQL how-to's
  warning: a follower serves the reports it restored at its start, never
  reloads them, and on a fresh database is never ready (ITEM-0114); a
  webhook sent to a follower waits in its memory (ITEM-0115). Known gap:
  between a step-down and its rejoin, a replica holds no serve lock, so a
  `db migrate` could slip in, only if that replica alone was cut off.
  `tcp_user_timeout` shows as `25000` (milliseconds) in `current_setting`.
- 2026-10-22: Verified: `make check` green; `make test-integration` green
  in 2 min 10 s, its largest process at 1.37 GiB (1.38 GiB before), with
  `TestServeOnPostgres` (two serves, a takeover after a stop),
  `TestLeaderStepsDown` (a stalled heartbeat; down within 10 s, with the
  ADR's own timings), and the store's `TestSessionSettings`,
  `TestReplicasShareTheServeLock`, `TestNoReplicaJoinsWhileLocked`,
  `TestElection` and `TestFence` on the lab's PostgreSQL; the leader tests
  passed three runs in a row. The takeover tests with child processes,
  killed and cut-off leaders are ITEM-0116's.
