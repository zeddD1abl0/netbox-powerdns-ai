---
id: ITEM-0080
title: The webhook docs and CHANGELOG
type: task # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M07
requirements: [REQ-048]
depends_on: [ITEM-0078, ITEM-0079]
created: 2026-10-08
closed: 2026-10-08
---

# ITEM-0080: The webhook docs and CHANGELOG

## Goal

The webhook docs: the how-to "Refresh drift as NetBox changes", the
service explanation's webhook section, "Run the development lab"'s worker
profile, "Service endpoints", the regenerated references, and the
CHANGELOG.

## Acceptance criteria

- [x] The how-to sets up NetBox by its UI, its API and Terraform, and troubleshoots a 401 and a 404.
- [x] The explanation, the lab how-to and "Service endpoints" cover webhooks.
- [x] The CHANGELOG has webhooks under Unreleased.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M07's approved design.
- 2026-10-08: The how-to "Refresh drift as NetBox changes" turns
  webhooks on, sets up the webhook and the event rule in NetBox's web
  interface, whose menu and field names were read from the lab's NetBox
  4.7, or with its REST API, checks them through `/api/status`, and fixes a
  401, a 404, a 400, and webhooks that never come.
- 2026-10-08: The plan said a Terraform example would create both objects.
  NetBox's provider, `e-breuninger/netbox`, has no argument for a webhook's
  secret, in its docs or its resource's schema, and nbpdns refuses
  unsigned webhooks. So the how-to creates the webhook by the UI or the API,
  and shows Terraform's `netbox_event_rule` given the webhook's ID.
- 2026-10-08: "How nbpdns runs as a service" gains "Webhooks from NetBox",
  and lost its stale lines that M07 would bring webhooks and M06 the RRset
  changes. "How nbpdns's API is designed", the API how-to, "Service
  endpoints" and "Run the development lab" cover the endpoint, its
  signature and the worker profile. The CHANGELOG names the how-to.
