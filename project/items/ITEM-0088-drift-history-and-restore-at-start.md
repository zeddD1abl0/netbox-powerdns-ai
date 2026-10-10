---
id: ITEM-0088
title: Drift history and restore at start
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-044, REQ-052]
depends_on: [ITEM-0087]
created: 2026-10-09
closed: 2026-10-10
---

# ITEM-0088: Drift history and restore at start

## Goal

The drift history in the database (ADR-0042): each refresh and each
group's result; a zone's row when its state or changes differ from its
last; each group's last report, restored at start, with `/readyz` and
`/status` saying so; a zone's change of state as an audit event; and the
prune by `drift.history_retention_days`.

## Acceptance criteria

- [x] A zone driven through drift and back has a row for each change, and a refresh with no change writes none.
- [x] After a restart, `serve` serves its last reports at once, and says how old they are.
- [x] A failed history write is counted, logged and shown, and the refresh is still served.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-09: Created from M08's approved design, on `plan-m08-m12`.
- 2026-10-10: Migration 3: `drift_refreshes`, `drift_refresh_groups`,
  `drift_zone_history` and `drift_group_reports`, and `sequences`, which
  hands out IDs, so that a prune never lets one be used again (MAX + 1
  would, once every refresh was pruned). `Store.RecordRefresh` writes a
  refresh in one transaction: the refresh, each group's result, a row for
  each zone whose state or changes' digest (the SHA-384 of its sorted
  changes) differs from its last row, a `removed` row for a zone that left
  the report, each group's report, and a `zone.drift_changed` event for
  each change of state. A new zone gets an event only if it drifted, so a
  database's first refresh doesn't record every zone in sync. Diffs are
  against the database's last rows, not a cache, so after a failed write
  the next refresh writes what changed since the last rows.
  `RestoreDrift` reads the last refresh, the last complete one, and each
  group's report and last result; warnings are kept beside the report,
  since a warning's JSON is only its text, and `drift.Merge` needs their
  zones. `PruneDriftHistory` deletes refreshes and zone rows older than the
  cutoff, but keeps each zone's last row unless it says `removed`.
- 2026-10-10: The service: `Options.History`; `Restore`, before `Run`,
  which restores the configured groups, the last refresh for display, and
  the drift metrics, and makes the service ready if any report came back;
  `keep`, after each full or zone refresh, with its kind, cause, NetBox
  request IDs and trace ID, on a context that outlives the refresh's
  timeout, counting a failure in `nbpdns_db_write_failures_total{op}`,
  logging it, and showing it in `/status`'s new `database` section.
  Liveness now counts from the start, not from a restored refresh, which
  could be days old. `drift.history_retention_days`, default 400; serve's
  daily prune covers the history too. "Service endpoints", the
  explanation's restart and readiness paragraphs, the API's `ready`, and
  the how-tos that describe readiness say what changed. The API's view of
  the history, and its `database` section, come with ITEM-0089.
- 2026-10-10: Done. `make check` passed; `make test-integration` passed
  (internal/cli 74.1 s, internal/netbox 12.0 s). ITEM-0093's integration
  test restarts serve on the lab, with real reports.
