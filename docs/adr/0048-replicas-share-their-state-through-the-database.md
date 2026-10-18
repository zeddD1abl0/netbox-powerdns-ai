---
title: '0048: Replicas share their state through the database'
status: accepted # proposed | accepted | rejected | deprecated | superseded by ADR-NNNN
date: 2026-10-18
decision-makers: [jordan]
requirements: [REQ-033, REQ-050, REQ-052]
questions: []
supersedes: # ADR-NNNN this replaces, if any
---

# 0048: Replicas share their state through the database

## Context and problem statement

With several replicas of `nbpdns serve` (ADR-0047), every one serves the
API and `/status`, and only the leader refreshes. A load balancer may send
each request to any replica, so each must answer the same.

Today each `serve` builds its answers from its own memory. Only drift
history is read from the store; each group's last report is written there,
but read back only at start (ADR-0042). NetBox's zones, which the API's
records come from, aren't stored at all. Webhooks' zones wait in memory
for their refresh (ADR-0036), so a webhook that reaches a replica other
than the leader is lost. Settings and groups changed with the CLI reach a
running `serve` through a 5-second poll of their versions (ADR-0040).

The user asked, on 2026-10-14, for both ways of learning of a change:
polling by default, with PostgreSQL's `LISTEN` and `NOTIFY` preferred.

## Decision drivers

- Every replica answers the same, whichever one a request reaches (M09).
- A webhook is refreshed whichever replica it reaches, and isn't lost to a
  restart or a takeover.
- Changes reach every replica within seconds (REQ-050), with polling as
  the default and `LISTEN` and `NOTIFY` as the preferred option.
- One behavior on both databases, wherever it costs little.
- The API stays fast: no large reads for each request.

## Considered options

1. **Learning of changes:** polling; `LISTEN` and `NOTIFY`; polling always,
   with `LISTEN` as an option that makes it immediate.
2. **Followers' answers:** read from the database for each request;
   memory, reloaded from the database when its version moves; requests
   forwarded to the leader.
3. **NetBox's records on a follower:** stored by the leader; read from
   NetBox by each follower; forwarded to the leader.
4. **Webhooks on a follower:** a queue in the database, on both databases;
   a queue in the database on PostgreSQL, and memory on SQLite; forwarded
   to the leader over HTTP.
5. **A refresh that can't be written:** served anyway, as in M08; served
   only once it's committed.

## Decision outcome

Chosen: **polling always, with `LISTEN` as an option; memory reloaded from
the database; NetBox's zones stored; a queue in the database on both; and
a refresh served once it's committed**.

- **The change feed:** every replica polls the database every
  `database.poll_interval`, 5 seconds by default, for the versions it
  follows: the settings, the groups, the last refresh, the refresh queue
  and the leader. It reloads whatever moved. This generalizes ADR-0040's
  settings watcher.
- **`LISTEN` and `NOTIFY`:** on PostgreSQL, every change sends `NOTIFY
  nbpdns` in its own transaction. With `database.listen: true`, a replica
  listens on a connection of its own, and polls at once when notified. It's
  off by default, as the user chose, and the docs recommend it where no
  transaction-mode pooler sits in between. Polling goes on either way, so a
  lost notification costs at most one interval.
- **Served from the database:** every replica's API and `/status` show what
  the database holds. A replica keeps it in memory, and reloads it when the
  last refresh moves. That covers each group's last report and the last
  refreshes. It also covers NetBox's zones, which the leader now stores,
  zone by zone, only when a zone's digest changes, so that a restarted
  replica serves the API's records at once too.
- **Commit before serving:** the leader serves a refresh once it's
  committed. If it can't be written, no replica serves it, and it's counted
  as a write failure, as now. This holds on SQLite too.
- **Status:** `/status`'s facts are of two kinds, and the API spec says
  which field is which. The deployment's come from the database and are
  the same on every replica: the last and next refresh, the groups,
  NetBox, the webhooks' queue and their last refresh, and the leader. The
  replica's are its own: its build, uptime, readiness, liveness, tracing,
  write failures, and the refreshes it ran.
- **Metrics:** every replica sets the drift gauges from what it has loaded.
  Counters and histograms count each replica's own work. The docs show how
  to aggregate across replicas, with `max by`.
- **Webhooks on any replica:** every replica, on either database, writes a
  webhook's zones to a refresh queue in the database. The leader gathers
  the queue, with ADR-0036's delay and bounds, and deletes the rows that
  its refresh covered once that refresh is recorded. ADR-0036 stands
  otherwise.
- **Readiness:** a follower is ready once it has loaded the reports, as a
  restored `serve` is (ADR-0042).
- **One config file:** replicas share one. Each mirrors its groups at start,
  as now (ADR-0040), so the last to start wins.

### Consequences

- Good: any replica answers any request the same, within one poll
  interval, or at once with `LISTEN`.
- Good: a webhook is refreshed whichever replica takes it, and survives a
  restart or a takeover, on SQLite too.
- Good: a restart serves NetBox's records at once, lifting M08's "NetBox's
  records aren't persisted".
- Good: the API reads memory, as now, so it stays fast.
- Bad: the database holds a copy of NetBox's zones, with their records,
  which grows with NetBox.
- Bad: a follower's answers lag the leader's by up to one poll interval
  without `LISTEN`.
- Bad: a refresh that can't be written is no longer served, on SQLite
  either, so a full disk stops the report from moving on. `/status` and
  the write-failure metric say so.
- Bad: each replica exposes the drift gauges, so dashboards and alerts
  aggregate them.

### Confirmation

- The takeover tests (ADR-0047) also send a webhook to a follower, and
  change a setting with the CLI, and check that every replica serves the
  result, with `database.listen` off and on.
- Contract tests on `/status` cover a leader and a follower.
- A test fails a refresh's write, and checks that no replica serves it.

## Pros and cons of the options

### Polling alone

- Good: works through any pooler.
- Bad: every change waits for the next poll.

### `LISTEN` and `NOTIFY` alone

- Good: immediate.
- Bad: a notification sent while a listener reconnects is lost, and a
  transaction-mode pooler drops them all.

### Reading the database for each request

- Good: nothing to reload.
- Bad: each request reads and decodes whole reports, which can be large.

### Forwarding requests to the leader

- Good: one source of answers.
- Bad: every replica depends on reaching the leader, and a takeover
  interrupts the API.

### Followers reading NetBox themselves

- Good: nothing more to store.
- Bad: each replica's records differ in age, and NetBox's load grows with
  the replicas.

### Memory on SQLite, the database on PostgreSQL

- Good: no write for a webhook on SQLite.
- Bad: two code paths, and SQLite's queued webhooks lost at a restart.

### Forwarding webhooks to the leader

- Good: no queue table.
- Bad: lost while the leader changes, and a new path between replicas,
  with its own authentication.

### Serving a refresh that can't be written

- Good: the leader's report moves on while the database fails.
- Bad: the leader and the followers answer differently.

## More information

- ADR-0036 (webhooks), ADR-0040 (runtime settings and groups), ADR-0042
  (drift history), ADR-0046 (the store on PostgreSQL), ADR-0047 (the
  leader).
- The brief, 2026-10-14: "polling being the default, but NOTIFY and LISTEN
  would be preferred".
- Recorded in M09's design, 2026-10-18, by ITEM-0109. Implemented by
  ITEM-0113, ITEM-0114 and ITEM-0115, and tested by ITEM-0116.
