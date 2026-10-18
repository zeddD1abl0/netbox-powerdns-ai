---
id: ITEM-0113
title: 'The replicas'' change feed: polling, and LISTEN and NOTIFY'
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: [REQ-033, REQ-050]
depends_on: [ITEM-0112]
created: 2026-10-18
closed:
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

- [ ] Settings and groups changed with the CLI reach every replica within the poll interval, or at once with `database.listen`.
- [ ] A lost notification, or a dropped `LISTEN` connection, costs at most one poll interval, and the listener reconnects.
- [ ] A leader's release is taken at the next poll, or at once with `database.listen`.
- [ ] On SQLite, the feed polls, and `database.listen` is refused with a clear error.
- [ ] `database.poll_interval` and `database.listen` are in the configuration reference, with a CHANGELOG line.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-18: Created from M09's approved design, at M09's start.
