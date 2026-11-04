---
id: ITEM-0113
title: 'The replicas'' change feed: polling, and LISTEN and NOTIFY'
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: [REQ-033, REQ-050]
depends_on: [ITEM-0112]
created: 2026-10-18
closed: 2026-11-04
---

# ITEM-0113: The replicas' change feed: polling, and LISTEN and NOTIFY

## Goal

The change feed (ADR-0048): one poll, every `database.poll_interval`
(5 seconds by default), of the versions that every replica follows; on
PostgreSQL, `NOTIFY nbpdns` in every change's transaction; and
`database.listen` (off by default), a `LISTEN` connection that makes a
replica poll at once. M08's settings watcher becomes the feed's consumer,
and followers try for the leader's lock at each poll.

## Acceptance criteria

- [x] Settings and groups changed with the CLI reach every replica within the poll interval, or at once with `database.listen`.
- [x] A lost notification, or a dropped `LISTEN` connection, costs at most one poll interval, and the listener reconnects.
- [x] A leader's release is taken at the next poll, or at once with `database.listen`.
- [x] On SQLite, the feed polls, and `database.listen` is refused with a clear error.
- [x] `database.poll_interval` and `database.listen` are in the configuration reference, with a CHANGELOG line.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-18: Created from M09's approved design, at M09's start.
- 2026-11-04: Done. `internal/store/feed.go`: `Versions` reads the
  settings' and the groups' versions and the leader's row in one read
  transaction; `InTx` ends each write transaction with `NOTIFY nbpdns` on
  PostgreSQL (a dialect field, so SQLite sends nothing); a `Listener` on a
  connection of its own, outside the pool's idle timeouts, with
  `idle_session_timeout` 0 and the replica's `application_name`; and
  `Session.Release`, which unlocks and notifies in one statement, so that
  a follower that hears it finds the lock free. `SettingsVersion` and
  `GroupsVersion` are gone. `internal/cli/feed.go`: one loop checks at
  start, every `database.poll_interval`, and when the listener pokes it;
  it hands each check to the election first, by a one-slot channel that
  the newest check replaces, then to the watcher's `apply`. Each read is
  bounded by 10 s, so a hung database doesn't stall a follower's tries
  and its `/livez`. The listener checks its connection after 30 s without
  a notification, and listens again a poll interval after a loss, then
  has the feed check. `leaderPoll` and `settingsPoll` are gone; a stopping
  leader releases before its session ends, and logs it. Config:
  `database.poll_interval`, 100ms to 30s (under `/livez`'s one-minute
  bound for followers), and `database.listen`, a strict `boolKey` (only
  `true` or `false`; viper would read `yes` as false), refused at load
  with a SQLite URL. The reference shows a boolean key's `false` default.
  Tests: `TestVersions` on both databases; `TestNotify`,
  `TestListenerOutlastsIdleTimeout`, `TestListenerLost` and `TestRelease`
  on PostgreSQL; `TestFeedCheck`; `TestFeedListens` (a terminated
  listener listens again); `TestServeListens` (two replicas, a 30 s poll:
  a setting and a takeover arrive within 5 s; it fails without the pokes,
  checked). The SQLite tests that waited on the 5 s poll use 100ms.
  `make check` passes; `make test-integration` passes in 1 min 44 s,
  its largest process at 1.38 GiB. Found ITEM-0120 while checking
  pipeline 845 for ITEM-0110.
