---
id: ITEM-0119
title: Say "1 event", not "1 events", in nbpdns audit verify
type: bug # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: [REQ-049]
depends_on: []
created: 2026-10-22
closed:
---

# ITEM-0119: Say "1 event", not "1 events", in nbpdns audit verify

## Goal

`nbpdns audit verify` reports a trail of one event as "The audit trail is
intact: 1 events, …" (`internal/cli/audit.go:121`), as a fresh database
has after its first change. The other messages with a count, such as
`nbpdns drift`'s "1 zone drifted", say the singular. Found on 2026-10-22,
checking ITEM-0111's backup how-to on PostgreSQL.

## Acceptance criteria

- [ ] A trail of one event is reported as "1 event", and a longer one as "N events"; a test covers both.
- [ ] Any docs that show the message still match it.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-22: Created from ITEM-0111.
