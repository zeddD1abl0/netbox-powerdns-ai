---
id: ITEM-0027
title: Avoid rebuilding the log handler chain per record for grouped loggers
type: debt # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M04
requirements: []
depends_on: []
created: 2026-10-06
closed: 2026-10-07
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

- [x] Logging through a grouped logger with bound attributes doesn't rebuild the handler chain per record. A benchmark shows it.
- [x] The IDs stay at the top level, as `TestLogLinesCarryIDs` and the logging tests check.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Found by `/code-review high` at M01's close (ITEM-0026,
  finding 8). Nothing groups loggers yet, so it costs nothing today. M04's
  service logs each HTTP request, which is when it starts to matter, so it's
  planned there. One approach: cache the rebuilt chain for the last set of
  IDs, since one request's records share them.
- 2026-10-07: Part of M04's approved design, in phase M4b.
- 2026-10-07: Done, a different way from the cache this item's first note
  suggested: a cache keyed by the IDs would miss on every span, and a
  refresh's requests each have their own span. Instead, `contextHandler`
  opens no group on its base handler. It keeps the groups itself, each
  with the attributes bound inside it, and writes each record's attributes
  into them as `slog.GroupValue`s, after the IDs, in one record to the base
  handler. Attributes bound before any group are still preformatted by the
  base handler's `WithAttrs`. A group left empty is written as no group, as
  slog's handlers do. `TestNestedGroups` checks two levels, a group that
  ends up empty, and a secret inside a group, still redacted.
  `BenchmarkLog`, with one group and five bound attributes, on a 16-thread
  workstation: 2.0 to 2.2 µs, 1,139 B and 18 allocations per record before;
  1.69 µs, 721 B and 9 allocations after. Without a group, about 0.97 µs
  and 4 allocations, unchanged. Attributes bound inside a group are still
  encoded with each record, as they were before, since slog's handlers can
  only preformat attributes outside every group; what's gone is the rebuilt
  handler chain and its allocations.
