---
id: ITEM-0027
title: Avoid rebuilding the log handler chain per record for grouped loggers
type: debt # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M04
requirements: []
depends_on: []
created: 2026-10-06
closed:
---

# ITEM-0027: Avoid rebuilding the log handler chain per record for grouped loggers

## Goal

`internal/logging`'s handler puts `trace_id`, `span_id` and `request_id` at
the top level of every record, even inside a log group. For a logger with a
group, or attributes added after one, it does that by rebuilding the whole
handler chain for every record: `base.WithAttrs(ids)`, then each
`WithGroup` and `WithAttrs` again. slog's handlers pre-format attributes in
`WithAttrs`, so each record re-encodes every bound attribute and allocates
new handlers. Make the cost per record independent of the logger's groups
and attributes.

## Acceptance criteria

- [ ] Logging through a grouped logger with bound attributes doesn't rebuild the handler chain per record. A benchmark shows it.
- [ ] The IDs stay at the top level, as `TestLogLinesCarryIDs` and the logging tests check.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Found by `/code-review high` at M01's close (ITEM-0026,
  finding 8). Nothing groups loggers yet, so it costs nothing today. M04's
  service logs each HTTP request, which is when it starts to matter, so it's
  planned there. One approach: cache the rebuilt chain for the last set of
  IDs, since one request's records share them.
