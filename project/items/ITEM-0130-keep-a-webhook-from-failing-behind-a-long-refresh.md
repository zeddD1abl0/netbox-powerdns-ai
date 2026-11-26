---
id: ITEM-0130
title: Keep a webhook from failing behind a long refresh's write
type: bug # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M10
requirements: [REQ-033, REQ-048]
depends_on: []
created: 2026-11-26
closed:
---

# ITEM-0130: Keep a webhook from failing behind a long refresh's write

## Goal

Since ITEM-0115, every replica writes a NetBox webhook's event, and the
refresh it asks for, to the queue in the database, in a write transaction.
On PostgreSQL, that transaction waits at most `busyTimeout`, 5 s, for the
write lock (`lockWrites`). The leader's `RecordRefresh` holds that lock for
its whole transaction. That transaction writes a row for each changed zone
and NetBox zone, one round trip each, so a first or large refresh, on a
database across the network, can hold the lock for longer. A webhook that
arrives then gets a 503. NetBox doesn't send it again, so its change waits
for the next scheduled refresh, up to `drift.interval` later. Before M09, a
webhook was queued in memory, and never failed this way.

## Acceptance criteria

- [ ] A webhook that arrives while the leader writes a large refresh, of
  thousands of changed zones, on PostgreSQL, is queued and answered 202,
  not refused.
- [ ] How long a webhook can wait to be written is bounded, and the bound,
  with how it compares with NetBox's webhook timeout, is recorded here and
  in the webhooks how-to.
- [ ] A test holds the write lock for longer than 5 s while a webhook
  arrives.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-11-26: Found by M09's `/code-review high`, on
  `main...m09-postgresql-and-ha`, and checked against the code: `Notify`
  in `internal/service/zones.go` calls `RecordWebhook`, whose transaction
  takes the write lock with `lock_timeout` at `busyTimeout`
  (`internal/store/postgresql.go`), and `internal/api/events.go` answers
  its failure with a 503. Left for after M09, as the user chose.
