---
id: ITEM-0026
title: Fix the M01 code review findings
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M01
requirements: [REQ-024, REQ-027, REQ-040]
depends_on: [ITEM-0023, ITEM-0024]
created: 2026-10-06
closed: 2026-10-06
---

# ITEM-0026: Fix the M01 code review findings

## Goal

Fix the findings of the `/code-review high` run at M01's close, on the
branch's whole diff against `main`. It found ten, none blocking: four
correctness problems in the NetBox client, one display bug, and five
cleanups. Nine are fixed here; one is deferred to ITEM-0027.

## Acceptance criteria

- [x] A next-page link keeps the path of `netbox.url`, so a proxy that strips a path prefix doesn't break page 2. Only the link's query is used. Covered by a test.
- [x] A network failure or timeout while reading an answer's body is an `UnreachableError`, and is retried. Covered by a test that stalls mid-body.
- [x] A list page's `count` can't crash nbpdns or make it reserve memory: negative and huge counts are handled. Covered by a test.
- [x] Lists are read in ID order, and a list whose count changes between pages, or doesn't match what arrived, is read again, up to three times, then fails. Covered by tests.
- [x] The `records` table shows an inactive record's own TTL, not its RRset's.
- [x] A `PermissionError` keeps the refusal's own detail, so a proxy's 403 isn't passed off as a missing NetBox permission. Covered by the integration test.
- [x] The supported-release text, the URL and token checks, and the list of object types each live in one place.
- [x] The deferred finding has an item, ITEM-0027.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: The review ran on `main...m01-netbox-read-path` at `a227636`.
  Findings and what became of each:
  1. `rebase` replaced the whole path, losing a proxy's prefix: fixed.
  2. Body read failures weren't retried: fixed. The body is read whole
     (pages are small) before it's decoded.
  3. `make([]T, 0, p.Count)` trusted the count: fixed, capped at 10,000.
  4. Offset paging could skip or repeat records when NetBox changes
     mid-read, which M03 would report as drift and M12 could act on: fixed,
     with `ordering=id` and a count check that rereads the list.
     `ordering=id` was checked against both lab releases.
  5. The table showed the RRset's TTL on inactive rows: fixed.
  6. Supported-release text built twice: fixed (`netbox.SupportedSeries`).
  7. URL and token checked twice: fixed; `netbox.New` returns
     `config.UnsetError`.
  8. The logging handler rebuilds its chain per record for grouped loggers:
     deferred to ITEM-0027. Nothing groups loggers yet, so it costs nothing
     today; M04's request logging is when it would.
  9. `ObjectTypes` repeated the endpoint map's keys: fixed, one ordered list.
  10. `denied` dropped the 403's detail: fixed.
  The tests that need a malformed or changing list serve the page recorded
  from the lab with only its `count` rewritten, so they still don't
  hand-write NetBox's API (ADR-0020).
