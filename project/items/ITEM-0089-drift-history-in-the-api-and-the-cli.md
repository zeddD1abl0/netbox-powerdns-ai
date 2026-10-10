---
id: ITEM-0089
title: Drift history in the API and the CLI
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-046, REQ-052]
depends_on: [ITEM-0088]
created: 2026-10-09
closed: 2026-10-10
---

# ITEM-0089: Drift history in the API and the CLI

## Goal

The history for readers: `GET /api/server-groups/{group}/zones/{zone}/history`
and `GET /api/server-groups/{group}/refreshes`, paged with cursors, in the
spec first; `nbpdns drift history`; and the API's version 1.2.0.

## Acceptance criteria

- [x] The operations answer as the spec says, each response checked against it.
- [x] `nbpdns drift history` shows a zone's history as text and JSON.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-09: Created from M08's approved design, on `plan-m08-m12`.
- 2026-10-10: Spec first: `listZoneHistory` and `listRefreshes`, with
  `HistoryState` (a `ZoneState` but `unmanaged`, or `removed`),
  `ZoneHistoryEntry`, `GroupResult` and `RefreshEntry`, their pages, and
  `Database` in `Status`; the API's version is 1.2.0, and vacuum scores it
  100. Both lists are newest first, and their cursors name the row or
  refresh they start before. A zone's history is served whether or not the
  zone is in the group's report now, since its rows outlive it, so a zone
  with none gets an empty page rather than a 404; an unknown group is a
  404. A refresh that didn't try the group, as one that couldn't read
  NetBox doesn't, has a null `group`. `api.Options.History` is the store;
  without one, the lists are empty. The store's readers are `ZoneHistory`
  and `Refreshes` (a left join on the group's results).
- 2026-10-10: `nbpdns drift history --group G [--zone Z] [--limit N]`
  lists the latest refreshes, or the zone's rows, oldest first, as a table
  or JSON, and refuses a database that isn't migrated. The how-to "Read a
  zone's drift history" covers both, and the API. The status's API
  mapping now carries `database`.
- 2026-10-10: Done. `make check` passed, vacuum 100/100; `make
  test-integration` passed (internal/cli 78.4 s).
