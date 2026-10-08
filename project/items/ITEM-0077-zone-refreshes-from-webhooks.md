---
id: ITEM-0077
title: Zone refreshes from webhooks
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M07
requirements: [REQ-044, REQ-048]
depends_on: [ITEM-0076]
created: 2026-10-08
closed: 2026-10-08
---

# ITEM-0077: Zone refreshes from webhooks

## Goal

Zone refreshes (ADR-0035): `drift.Options.Zones` compares a set of
zones; the service queues the zones that webhooks name, waits for
`drift.webhook_delay` of quiet, or 30 s, and compares only those zones,
or makes a full refresh past 100 zones or for a view; the results merge
into the kept state, metrics and NetBox's records; and each zone refresh
is a trace linked to its webhooks, with NetBox's request IDs and users.

## Acceptance criteria

- [x] A burst makes one zone refresh after the quiet spell, or a full one past 100 zones or for a view, and zone and scheduled refreshes never overlap, as tests show.
- [x] A zone refresh replaces only those zones' entries, counts, metrics and records; a zone gone from both sides leaves; a failing group keeps its report.
- [x] The zone refresh's trace links to the webhook spans, and carries NetBox's request IDs and users, as its logs do.
- [x] `drift.webhook_delay` and the zone-refresh metrics are declared, and the references regenerated.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M07's approved design.
- 2026-10-08: `drift.Options.Zones` names zones by view and name. Each
  group compares the names of those in its views, in every one of its
  views, so that the result is what a full comparison gives for them; a
  group with none is left out of the report. NetBox is listed once, by
  name (`name__ie`, ITEM-0081); a primary looks one name up, or lists every
  zone, one request, and keeps the names. `drift.Merge` replaces the
  compared names' zones, unmanaged zones, problems and warnings in the
  group's last report, with new slices, and counts again. A property test
  changes both sides and checks that the last full report, merged with the
  zone refresh, equals a new full report. Warnings became `drift.Warning`,
  with the zone each is about, still a string in JSON.
- 2026-10-08: The service queues webhooks' zones (`Notify`, which
  implements `api.Notifier`), leaving out views that no group serves. Its
  loop waits for the schedule, or for the queue: `drift.webhook_delay`
  (100 ms to 30 s, default 3 s) after the last webhook, or 30 s after the
  first. A view, more than 100 zones, or a moved zone or record makes a
  full refresh, which restarts the schedule. A scheduled refresh takes the
  whole queue, which it covers. Refreshes share the loop, so they never
  overlap. A zone refresh merges copy-on-write into the kept reports and
  NetBox's view, only where a full refresh kept one; a failed one drops its
  zones until the next scheduled refresh. Each is a "drift zone refresh"
  trace, linked to the webhook spans, with the zones, NetBox's request IDs
  and users; a full refresh that covers webhooks links to them too. Its
  log lines carry `netbox_request_ids`. The last event and the last
  webhook refresh are kept for ITEM-0078's status.
- 2026-10-08: `nbpdns serve` passes `netbox.webhook_secret` and the
  service to the API, so webhooks work from this commit. The CHANGELOG
  says so. The replayed integration test is ITEM-0079's.
