---
id: ITEM-0076
title: The webhook endpoint
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M07
requirements: [REQ-046, REQ-048]
depends_on: [ITEM-0075]
created: 2026-10-08
closed: 2026-10-08
---

# ITEM-0076: The webhook endpoint

## Goal

`POST /api/netbox-events`, which accepts NetBox's signed webhooks
(ADR-0035): the signature, checked in constant time, against
`netbox.webhook_secret`; the event decoded into the zones it names, by
view, or a request for a full refresh; the spec operation and its
security scheme; and `nbpdns_netbox_webhooks_total`.

## Acceptance criteria

- [x] `internal/webhook` checks signatures and maps every captured NetBox 4.7 event to its zones, a view event to a full refresh, and other types to nothing, as table tests show.
- [x] The endpoint answers 202, 401, 400, 404 when off, and 413, as the spec says, each checked against it.
- [x] `netbox.webhook_secret` is declared, with `_FILE`, and the references are regenerated.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M07's approved design.
- 2026-10-08: Captured 14 more NetBox 4.7 deliveries from the lab, with a
  temporary worker (`manage.py rqworker`) and a webhook and event rule made
  through the API, all removed afterwards: view, zone and record creations,
  updates and deletions, a zone renamed, a zone moved to another view, a
  record moved to another zone, a view renamed, an update the plugin made
  without a snapshot, and an `extras.tag`. They're in
  `internal/webhook/testdata/netbox-47/`, byte for byte, with the
  signatures NetBox sent, which `Sign` reproduces. `.editorconfig` keeps
  editors from adding a final newline to them.
- 2026-10-08: `snapshots.prechange` names related objects by ID only, so a
  zone moved to another view, or a record moved to another zone, makes a
  full refresh: the event can't name where it was. That covers the old
  place too, as ADR-0035 means a zone event to. A rename within a view
  refreshes the old and the new name.
- 2026-10-08: The spec's `NetBoxEvent` maps by `x-go-type` to
  `webhook.Event`, so the generated server decodes only what nbpdns reads.
  The signature is checked by a generated-server middleware on the routes
  whose spec `security` names `netboxSignature`, before the strict server
  decodes anything: a forged request with a body that isn't JSON is a 401,
  not a 400. `netbox.webhook_secret` must be at least 16 characters.
  `api/ruleset.yaml`'s [243] list gains 413, an official code. The 405
  problem no longer says that the API only reads. The reference page now
  shows each operation's signature and request body.
- 2026-10-08: `nbpdns serve` doesn't pass the secret to the API yet: the
  service's queue, which implements `api.Notifier`, comes with ITEM-0077,
  and so does the CHANGELOG line.
