---
id: ITEM-0049
title: Compare server groups concurrently
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M04
requirements: [REQ-043]
depends_on: []
created: 2026-10-06
closed: 2026-10-07
---

# ITEM-0049: Compare server groups concurrently

## Goal

`drift.Run` connects to, lists and reads each server group's primary in
turn, as M03's approved design says. With several groups, a run takes the
sum of their times, so one slow primary delays every other group's report.
Compare groups concurrently, with a bound, once the service (M04) runs
drift on a schedule and many groups make the wait matter. Found by M03's
`/code-review high`.

## Acceptance criteria

- [x] Groups are listed, read and compared concurrently, with a configurable bound, and the report's order stays the configuration's.
- [x] A test shows a slow group doesn't delay the others' reads.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-07: Part of M04's approved design, in phase M4c.
- 2026-10-07: Done. `drift.Run` takes `Options`, with the zone and the
  concurrency, which `drift.group_concurrency` (1 to 32, default 4) sets.
  Listing the primaries, then reading and comparing each group, run up to
  that many groups at once; NetBox is still listed and read once, between
  the two. Each group's result goes in its own slot, so the report keeps
  the configuration's order. `TestRunConcurrently` checks that at most the
  limit run at once, at 1 and at 2, and that a group whose read blocks
  until another group's read has finished doesn't deadlock. One key covers
  both `nbpdns drift` and `serve`. `intKey` now takes only an upper bound,
  since every integer key starts at 1, which `unparam` pointed out once a
  fourth key did.
