---
id: ITEM-0047
title: Mark enforce zones in the drift report
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M03
requirements: [REQ-031]
depends_on: [ITEM-0044]
created: 2026-10-06
closed: 2026-10-06
---

# ITEM-0047: Mark enforce zones in the drift report

## Goal

ADR-0027 says the report marks `enforce` zones as acting from M12, since
nothing is written until then. The JSON gives each zone's `policy`, but the
table shows no policy at all, so a reader can't tell which drifted zones
nbpdns will correct, or that it doesn't yet.

## Acceptance criteria

- [x] The table's drift section has a `POLICY` column, which shows `enforce` as `enforce (from M12)`.
- [x] Unit and integration tests check the column.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Found while writing ITEM-0046's docs against ADR-0027. The
  drift table's drift section now has a `POLICY` column after `ZONE`, with
  `enforce` written as `enforce (from M12)`; the JSON already had each
  zone's `policy`. `TestWriteDrift` checks both, and the integration test
  gives the fixture's drift zone `enforce`, checking it in the JSON and the
  table.
