---
id: ITEM-0126
title: End a lost leader's open transaction, so that it can't hold the write lock
type: bug # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: [REQ-033]
depends_on: [ITEM-0116]
created: 2026-11-25
closed:
---

# ITEM-0126: End a lost leader's open transaction, so that it can't hold the write lock

## Goal

ADR-0047 bounds how long a lost leader keeps the leader's lock: its
session's keepalives, `tcp_user_timeout` and `idle_session_timeout` end
it within about 30 seconds. But every write transaction also takes the
store's write lock, `pg_advisory_xact_lock` (ADR-0046), on a connection of
the store's pool, which sets none of those. A leader cut off, or frozen,
between a write transaction's start and its end leaves PostgreSQL holding
that transaction open, and with it the write lock: for a frozen process
or a half-open connection, whose kernel answers the keepalives, until it
resumes; across a lost network, until the server's keepalives, 7200
seconds by the OS's default, end it. Meanwhile a follower that takes the
leader's lock can't write its takeover (each try waits `busyTimeout`, 5
seconds, then fails), and no replica, nor the command line, can write
anything. The lab's PostgreSQL 18 has every such timeout at 0.

## Acceptance criteria

- [ ] A test, with ITEM-0116's proxy, cuts a replica off inside a write transaction that holds the write lock, and another replica takes over and writes within ADR-0047's bounds.
- [ ] The pool's connections bound an open transaction whose client is lost, as the replica's session does for its locks; the bound is chosen and recorded (an ADR if it changes ADR-0047's timings).
- [ ] A leader that resumes after its transaction was ended doesn't commit it.
- [ ] The reference docs and the CHANGELOG say so, if users see it.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-11-25: Found while writing ITEM-0116's cut-off case. That test cuts
  the leader off once its refresh is recorded, while its pool runs only the
  feed's read transactions, which take no advisory lock, so it doesn't meet
  this. Candidates: `idle_in_transaction_session_timeout` (and perhaps the
  keepalives) on every pool connection, through pgx's `RuntimeParams` or
  an `AfterConnect`, without overriding what the user's URL sets; and
  PostgreSQL 17's `transaction_timeout`. The proxy is in
  `internal/cli/takeover_integration_test.go`; a store test would need it
  moved where both packages reach it, such as `internal/lab`.
