---
id: ITEM-0106
title: Status mixes a running refresh's start with the last one's end
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-046]
depends_on: []
created: 2026-10-15
closed: 2026-10-15
---

# ITEM-0106: Status mixes a running refresh's start with the last one's end

## Goal

While a refresh runs, `/status` and `/api/status` show as the last refresh
the running one's start with the previous one's end, so its duration is
negative, which breaks the API's contract (`minimum: 0`). It has been so
since `/status` came, in M04, for every refresh but the first, unnoticed
because refreshes are short. Since M08's restore, the first refresh after a
restart runs with the restored one as the last, so it shows every time,
which ITEM-0093's restart test found. The last finished refresh's start is
kept apart from the running one's.

## Acceptance criteria

- [x] While a refresh runs, the status's last refresh is the one before
  it, with its own start and a duration that isn't negative; a unit test
  shows it, and fails without the fix.
- [x] After a restore, the last refresh is the restored one, start and
  end.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-15: Found by ITEM-0093's `TestServeRestarts`, whose
  `/api/status` right after a restart failed the contract check with
  `duration_seconds` -0.73. `service.state` keeps `lastStart`, which a
  running refresh moves and which liveness, the schedule and "is one
  running" read, and now `lastEndStart`, the start of the refresh that
  finished at `lastEnd`, which only the status reads. Fixed in the same
  session as ITEM-0093, since it blocked that item's test.
- 2026-10-15: Done. `TestStatusDuringARefresh` reads the status while a
  second refresh runs, and failed without the fix with a duration of
  -0.0003 s; the restore test also checks that the restored last refresh
  doesn't start after it ended. The CHANGELOG has a line under Fixed.
  `make check` passed.
