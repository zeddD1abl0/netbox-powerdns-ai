---
id: ITEM-0132
title: Don't count a webhook whose sender left as a database write failure
type: bug # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M10
requirements: [REQ-048]
depends_on: []
created: 2026-11-26
closed:
---

# ITEM-0132: Don't count a webhook whose sender left as a database write failure

## Goal

`Notify` writes a webhook to the database with the request's context.
Suppose NetBox's request ends, such as at its timeout, while the write
waits for the write lock. The write then fails with `context canceled`, and
nbpdns counts it in `nbpdns_db_write_failures_total{op="webhook"}`, and logs
an error that the database couldn't be written. That can set off a
database alert while the database is fine.

## Acceptance criteria

- [ ] A webhook whose sender has gone before its write finishes isn't
  counted or logged as a database write failure. Either it's written all
  the same, within a bound of its own, or it's counted and logged as the
  sender having left.
- [ ] A test cancels a webhook's request while its write waits.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-11-26: Found by M09's `/code-review high`, as a nit, on
  `main...m09-postgresql-and-ha`, and checked against the code: `Notify`
  in `internal/service/zones.go` passes the request's `ctx` to
  `RecordWebhook`, and counts any error. Left for after M09, as the user
  chose.
