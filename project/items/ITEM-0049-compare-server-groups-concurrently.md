---
id: ITEM-0049
title: Compare server groups concurrently
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M04
requirements: [REQ-043]
depends_on: []
created: 2026-10-06
closed:
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

- [ ] Groups are listed, read and compared concurrently, with a configurable bound, and the report's order stays the configuration's.
- [ ] A test shows a slow group doesn't delay the others' reads.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
