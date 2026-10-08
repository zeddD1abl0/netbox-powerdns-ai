---
id: ITEM-0082
title: Fix the M07 code review findings
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M07
requirements: [REQ-048]
depends_on: []
created: 2026-10-08
closed: 2026-10-08
---

# ITEM-0082: Fix the M07 code review findings

## Goal

`/code-review high` on M07's branch, on 2026-10-08, found nine issues.
Six are fixed here; three are declined, each with its reason below.

## Acceptance criteria

- [x] Each finding is fixed, with a test, or declined with a reason in the notes.
- [x] `make check`, `make test-integration` and `make test-webhooks` pass.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Fixed:
  1. A zone refresh listed up to 100 names in one NetBox request, whose URL
     could pass a proxy's limit, 8 KiB in nginx by default, and read as
     NetBox down. `netbox.Client.Zones` now asks for 20 names a list; a
     test checks the chunks, and that 20 of the longest DNS names stay
     under 6 KiB.
  2. and 3. A zone refresh moved its group's `last_success`, and
     `nbpdns_server_group_last_success_timestamp_seconds`, though most of
     the report was older; a group whose full refreshes failed could look
     fresh. Only a full comparison moves them now, as NetBox's records'
     `as_of` already did. The spec, the metric and "Service endpoints" say
     that zones that webhooks named may be newer.
  4. Every view event made a full refresh, even for a view that no group
     serves. `webhook.Refresh` now carries a view event's names, the new
     and the old, and `Notify` ignores the event unless a group serves one.
  5. `nbpdns_drift_pending_zones` counted the zones gathered once a full
     refresh waited, while `/status` showed none. `/status` shows them now
     too, documented as covered by the full refresh.
  6. The reference generator would panic on a security scheme that isn't
     an API key, which M10's tokens may be. It names the scheme then.
- 2026-10-08: Declined:
  - A failed zone refresh drops its zones, rather than queuing them again.
    ADR-0035 makes the scheduled refresh the safety net, which the docs
    say; retrying would keep calling a NetBox that's down, every
    `drift.webhook_delay`. A durable queue, with retries, can come with the
    database, in M08.
  - The merges run under the service's lock. A merge into a 20,000-zone
    report took 3.8 ms (a benchmark, not kept), twenty times the scale that
    M06 measured, once a zone refresh, which comes at most every few
    seconds.
  - Each refused webhook logs a warning, unlimited. Every request to the
    API already logs a line, so an attacker who can reach it can already
    fill the log at that rate; the warning names the cause for an operator
    whose secrets differ.
