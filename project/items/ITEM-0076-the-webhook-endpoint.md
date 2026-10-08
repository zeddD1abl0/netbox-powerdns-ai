---
id: ITEM-0076
title: The webhook endpoint
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M07
requirements: [REQ-046, REQ-048]
depends_on: [ITEM-0075]
created: 2026-10-08
closed:
---

# ITEM-0076: The webhook endpoint

## Goal

`POST /api/netbox-events`, which accepts NetBox's signed webhooks
(ADR-0035): the signature, checked in constant time, against
`netbox.webhook_secret`; the event decoded into the zones it names, by
view, or a request for a full refresh; the spec operation and its
security scheme; and `nbpdns_netbox_webhooks_total`.

## Acceptance criteria

- [ ] `internal/webhook` checks signatures and maps every captured NetBox 4.7 event to its zones, a view event to a full refresh, and other types to nothing, as table tests show.
- [ ] The endpoint answers 202, 401, 400, 404 when off, and 413, as the spec says, each checked against it.
- [ ] `netbox.webhook_secret` is declared, with `_FILE`, and the references are regenerated.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M07's approved design.
