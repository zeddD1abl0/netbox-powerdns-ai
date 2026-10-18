---
title: '0047: Elect the leader with a PostgreSQL session advisory lock'
status: accepted # proposed | accepted | rejected | deprecated | superseded by ADR-NNNN
date: 2026-10-18
decision-makers: [jordan]
requirements: [REQ-033]
questions: [Q-062]
supersedes: # ADR-NNNN this replaces, if any
---

# 0047: Elect the leader with a PostgreSQL session advisory lock

## Context and problem statement

On PostgreSQL, several replicas of `nbpdns serve` run at once, and exactly
one of them refreshes, prunes and, from M14, writes to PowerDNS (REQ-033).
ADR-0009 intended a PostgreSQL advisory lock for that, and the user chose
it on 2026-10-09 (Q-062). This ADR settles how the lock is held, how long a
lost leader keeps it, and how a leader that has lost it is kept from
acting.

A session advisory lock lasts as long as the session that took it.
PostgreSQL ends a session when the client closes it, or when it finds the
client gone. A host that dies, or a network that drops, closes nothing, so
by default the session, and the lock, can last for hours. A process that
hangs keeps its session alive: its kernel still answers TCP keepalives.
And a leader cut off from the database may not know it has lost the lock
when another replica takes it.

## Decision drivers

- Exactly one leader at a time, never two, even briefly (REQ-033).
- Another replica takes over within about 30 seconds of the leader's loss
  (M09's goal).
- No infrastructure beyond PostgreSQL (ADR-0009).
- A takeover is visible: in `/status`, the metrics and the audit trail.

## Considered options

1. **The mechanism:** a session advisory lock; a lease row with an expiry
   time; Kubernetes' Lease API; etcd or Consul.
2. **Bounding a lost leader's lock:** TCP keepalives alone; keepalives,
   `tcp_user_timeout` and `idle_session_timeout`.
3. **Keeping a deposed leader from acting:** timing alone; a check of the
   lock in each leader-only write; leader-only writes through the lock's
   own connection.

## Decision outcome

Chosen: **a session advisory lock, bounded by keepalives, the user timeout
and the idle-session timeout, with a heartbeat, a step-down, and a check
of the lock in leader-only writes**.

- **The lock:** `pg_try_advisory_lock`, on a key derived from the
  database's ID (ADR-0038), so that two installs never share one. It's held
  on a connection of its own, which never goes through a transaction-mode
  pooler.
- **The bound:** the lock's session sets `tcp_keepalives_idle=10`,
  `tcp_keepalives_interval=5` and `tcp_keepalives_count=3`, for a dead
  host; `tcp_user_timeout=25s`, for data the leader never acknowledges; and
  `idle_session_timeout=30s`, for a hung process. PostgreSQL then ends a
  dead leader's session, and with it the lock, within about 25 seconds, and
  a hung one's within 30.
- **Heartbeat and step-down:** every 5 seconds, the leader updates its row
  on the lock's connection, with a 5-second deadline; that also keeps the
  session from idling. A leader that misses one steps down: it cancels its
  refresh, stops the prune, closes the connection, and follows. It steps
  down within 10 seconds, before PostgreSQL can free the lock, so two
  leaders never overlap.
- **The fence:** leader-only writes check, in their transaction, that the
  lock's session, by its backend PID in `pg_locks`, still holds the lock.
  A deposed leader that hasn't yet noticed, such as a hung process that
  resumes, can't write. These writes are a refresh's history, the prune,
  the refresh queue's drain (ADR-0048), and, from M14, the write path's
  records.
- **Leader-only work:** refreshes and the prune, and, from M14, the writes.
  Followers try for the lock at each poll of ADR-0048's change feed. A
  stopping leader releases its lock, and a follower takes it at its next
  poll, or at once with `LISTEN`.
- **The leader's row:** written under the lock. It holds the replica's
  name (its host name), a random ID from its start, when it took over,
  when it was last seen, and when its next refresh is due.
- **Visibility:** a takeover is a `service.leader_acquired` audit event,
  with the previous leader, and when it was last seen, as its before.
  `/status` gains `replica` and `leader`, and the metrics
  `nbpdns_leader`, `nbpdns_leader_since_timestamp_seconds` and
  `nbpdns_leader_changes_total`.
- **SQLite:** `serve` always leads, under ADR-0038's `flock`, with no lock
  and no leader events.

### Consequences

- Good: no new infrastructure, and PostgreSQL itself frees a lost leader's
  lock, dead or hung, within about 30 seconds.
- Good: the leader steps down before its lock can pass on, and the fence
  catches the case that timing can't, a process that resumes after a
  pause.
- Good: every replica can say who leads, since when, and when it was last
  seen.
- Bad: the fence covers the database's writes only. M14's writes to
  PowerDNS must check the leadership just before each write, and a
  leader's write that is already in flight when it loses the lock can
  still land.
- Bad: a leader whose heartbeat is slow, under load or in a long garbage
  collection, steps down needlessly, and a refresh is lost to the
  takeover.
- Bad: each replica holds a connection of its own, outside the pool, and a
  transaction-mode pooler can't sit in front of it.

### Confirmation

- The takeover tests run three replicas as child processes, on a lab
  database, and check that exactly one refreshes; that a stopped or killed
  leader is replaced within 5 seconds; and that a leader cut off by a TCP
  proxy steps down before another takes over, within 40 seconds.
- A test checks the lock session's settings, and that a write fails the
  fence once its lock is gone.

## Pros and cons of the options

### A lease row with an expiry time

- Good: works through any pooler, and on any database.
- Bad: the replicas' clocks must agree, and a lease is only as good as its
  renewal; the database can't end it for a dead holder.

### Kubernetes' Lease API

- Good: Kubernetes' own mechanism, which its controllers use.
- Bad: ties HA to Kubernetes, which ADR-0009 doesn't assume, and adds an
  API permission to every deployment.

### etcd or Consul

- Good: built for this.
- Bad: infrastructure beyond PostgreSQL, which ADR-0009 rules out.

### TCP keepalives alone

- Good: one mechanism.
- Bad: a hung process's kernel answers the keepalives, so its lock lasts
  until it's killed.

### Timing alone

- Good: no check in each write.
- Bad: a process paused past its step-down, then resumed, writes as the
  leader.

### Leader-only writes through the lock's connection

- Good: a lost lock means a dead connection, so the write fails by itself.
- Bad: every leader-only write, and the heartbeat, queue on one connection,
  and the store needs a second way to run a transaction.

## More information

- ADR-0009 (persistence and HA), ADR-0038 (the database's ID and the
  `flock`), ADR-0046 (the store on PostgreSQL), ADR-0048 (the change feed).
- PostgreSQL 18's documentation: advisory locks, and the connection
  settings `tcp_keepalives_*`, `tcp_user_timeout` and
  `idle_session_timeout`.
- Recorded in M09's design, 2026-10-18, by ITEM-0109. Implemented by
  ITEM-0112, and tested by ITEM-0116.
