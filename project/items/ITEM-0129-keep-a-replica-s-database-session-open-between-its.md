---
id: ITEM-0129
title: Keep a replica's database session open between its checks and while it starts
type: bug # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M10
requirements: [REQ-033]
depends_on: []
created: 2026-11-26
closed:
---

# ITEM-0129: Keep a replica's database session open between its checks and while it starts

## Goal

Each replica's session holds its locks: the serve lock, shared, and on the
leader, the leader's lock. It sets `idle_session_timeout` to 30 s, so that
PostgreSQL ends a hung replica's session (ADR-0047). But a follower uses
the session only to try for the leader's lock at each of the change feed's
checks, every `database.poll_interval`, which may be up to 30 s. And
`serve` joins before it migrates and restores, which can take longer, such
as while it waits for another replica's migration (`openServing`, then
`s.migrate`, in `internal/cli/serve.go`). Either
way, PostgreSQL ends a healthy replica's session. The replica then logs
`couldn't try for the leader's lock`, and joins again at its next check.
Until then it holds no serve lock, so `db migrate` or `secrets
rotate-master-key` could run beside it, and its takeover is delayed.

## Acceptance criteria

- [ ] A follower keeps its session while it runs, at every allowed
  `database.poll_interval`, up to 30 s.
- [ ] A replica whose start takes longer than 30 s before its first check
  keeps its session, and with it the serve lock, from its start to its stop.
- [ ] PostgreSQL still ends a hung leader's session, and frees its lock,
  within about 30 s, as ADR-0047 has it, or a new ADR records the change.
- [ ] Tests cover a follower at a 30 s poll and a start slower than 30 s.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-11-26: Found by M09's `/code-review high`, on
  `main...m09-postgresql-and-ha`, and checked against the code: the
  session's settings are in `internal/store/leader.go`'s `sessionSettings`,
  and `database.poll_interval`'s range is 100 ms to 30 s
  (`internal/config/keys.go`). Left for after M09, as the user chose.
