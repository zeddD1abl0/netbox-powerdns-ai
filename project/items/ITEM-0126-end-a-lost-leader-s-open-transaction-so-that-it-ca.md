---
id: ITEM-0126
title: End a lost leader's open transaction, so that it can't hold the write lock
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: [REQ-033]
depends_on: [ITEM-0116]
created: 2026-11-25
closed: 2026-11-25
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

- [x] A test, with ITEM-0116's proxy, cuts a replica off inside a write transaction that holds the write lock, and another replica takes over and writes within ADR-0047's bounds.
- [x] The pool's connections bound an open transaction whose client is lost, as the replica's session does for its locks; the bound is chosen and recorded (an ADR if it changes ADR-0047's timings).
- [x] A leader that resumes after its transaction was ended doesn't commit it.
- [x] The reference docs and the CHANGELOG say so, if users see it.

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
- 2026-11-25: Done, as ADR-0049 records, which is `proposed` until the
  user agrees to it. `lockWrites` (`internal/store/postgresql.go`) sets
  `idle_in_transaction_session_timeout` to 20 seconds, `writeIdleTimeout`,
  with `set_config(..., true)` in the statement that already set
  `lock_timeout`, so only write transactions, the ones that hold the write
  lock, are bounded, at no extra round trip. Reads, the pool's
  connections between writes, and the URL's settings keep theirs
  (`TestSessionSettings` still sees the pool's defaults). 20 seconds is no
  later than a lost leader's session ends, 20 to 30 seconds after the
  loss.
  - `TestLeaderLostInAWrite` (`internal/store`, integration): a leader cut
    off inside a leader-only write, through the proxy. Before the fix it
    failed: no other write within 25 seconds. After it, the other replica
    wrote 20.0 s after the cut, took over at 30.1 s, and wrote as the
    leader; the lost leader's write, once the proxy was restored, failed,
    and its event isn't in the trail. It adds about 30 s to
    `internal/store`'s integration tests.
  - The proxy moved from `internal/cli`'s takeover test to
    `internal/lab/proxy.go` (`lab.NewProxy`, `lab.Proxied`), which only
    tests import, and gained `Restore`: while cut off it holds what it
    read, and forwards it once restored. `TestTakeovers` uses it, with its
    times unchanged.
  - Docs: "How nbpdns stores its state" has a bullet on the write lock and
    links ADR-0049; "Run nbpdns on PostgreSQL" says a lost replica's write
    ends within 20 seconds; the CHANGELOG's M09 line on replicas says so
    too, rather than a Fixed line, since no release had the bug.
  - `make check` and `make test-integration` pass. `internal/cli`'s
    integration tests took 222 s, against 147 s that morning: the
    committed code, run then, was as slow, so it's the lab's pace, not
    this change.
