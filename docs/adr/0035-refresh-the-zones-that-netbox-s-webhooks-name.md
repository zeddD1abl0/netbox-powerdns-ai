---
title: "0035: Refresh the zones that NetBox's webhooks name"
status: superseded by ADR-0036
date: 2026-10-08
decision-makers: [jordan]
requirements: [REQ-044, REQ-046, REQ-048]
questions: [Q-037, Q-054]
supersedes:
---

# 0035: Refresh the zones that NetBox's webhooks name

## Context and problem statement

`nbpdns serve` refreshes the drift report every `drift.interval`, 5 minutes
by default (ADR-0029). So a change in NetBox shows as drift only after the
next refresh, which at 1,000 zones takes about a minute of reads against
NetBox and every primary. M13's writes will need to follow NetBox's
changes as they happen.

NetBox can send a webhook when an object changes: an event rule names the
object types and events, and a webhook names where to send them. In the
lab's NetBox 4.7, on 2026-10-08:
- **The payload:** each delivery is JSON with `event` (`created`,
  `updated` or `deleted`), `object_type` (such as `netbox_dns.record`),
  `timestamp`, `request` (NetBox's request `id` and `user`, among others),
  `data` (the object, as NetBox's API serializes it), and `snapshots`.
- **A record names its zone and view;** so does a zone.
- **The signature:** `X-Hook-Signature` is the hex HMAC-SHA512 of the raw
  body, keyed by the webhook's secret.
- **Bursts:** one API call sends several events, sharing its request ID,
  and a bulk edit sends thousands.
- **No `traceparent`** is sent.
- **Delivery needs NetBox's worker**, which the development lab doesn't
  run.

Q-054 asked what triggers a sync: a webhook, plus the periodic refresh.
Q-037 asked how far traceability goes. The user answered both on
2026-10-08.

## Decision drivers

- The zones a change touches are known from the event. A full refresh for
  each burst would cost a minute of reads at scale.
- A webhook can be lost, so the scheduled refresh must stay.
- The endpoint must refuse anything NetBox didn't sign, while the rest of
  the API has no authentication (ADR-0033).
- nbpdns keeps its read-only NetBox token (ADR-0023).
- CI's runners are short of memory: the integration job has been killed
  under memory pressure.

## Considered options

1. **What a webhook refreshes:** only the zones it names; a full refresh,
   sooner.
2. **Traceability now:** NetBox's request ID and user; those, and the
   change IDs from NetBox's changelog.
3. **Testing:** replayed, signed payloads in CI, with a NetBox worker on
   demand in the lab; a worker in CI's lab; replay only.
4. **Setup in NetBox:** by the user, from nbpdns's docs; registered by
   nbpdns; printed by nbpdns.

## Decision outcome

The user chose the first option of each on 2026-10-08.

**The endpoint:** `POST /api/netbox-events`, in the OpenAPI document, under
the API's middleware.
- **Off by default:** it's off unless `netbox.webhook_secret` is set (a
  secret, with a `_FILE` form). Until then it's a 404 problem saying how to
  turn it on.
- **Signed:** every request needs a valid `X-Hook-Signature`, compared in
  constant time. Without one, it's a 401 problem, and nothing is read from
  the body.
- **Bounded:** bodies are limited to 1 MiB. A malformed event is a 400.
- **Fast:** an accepted event gets a 202, at once.
- **In the spec:** the signature is the operation's own security scheme.
  The API's version becomes 1.1.0.

**Events to zones:**
- a `netbox_dns.record` event names its zone, in the zone's view;
- a `netbox_dns.zone` event names the zone, and, from `snapshots.prechange`,
  its old name and view too;
- a `netbox_dns.view` event asks for a full refresh;
- other types, and zones in views that no group serves, are ignored and
  counted.

**Zone refreshes:**
- Zones gather until no event has come for `drift.webhook_delay`, default
  3 s, or for 30 s since the first, whichever is sooner. Then the groups
  that serve each zone's view compare only those zones, NetBox's records
  for them included.
- More than 100 zones, or a view event, makes a full refresh instead.
- Zone and scheduled refreshes share the service's refresh loop, so they
  never overlap. A scheduled refresh covers any zones waiting.
- A zone refresh replaces those zones' entries in each group's report, the
  group's counts, the drifted-zone metrics, and NetBox's records. Every kept
  report is a new copy. A zone that neither side has any more leaves the
  report. A group whose primary fails keeps its last report, and is marked
  failed.

**Traceability (Q-037), now:**
- each webhook's span and log line carry NetBox's request ID, user, event
  and object type;
- each zone refresh is a trace of its own, linked to the webhook spans it
  serves, with the zones, request IDs and users as attributes.

The rest of Q-037's chain (apply, each server, verification), and NetBox's
change IDs, come with writes in M13.

**Metrics and status:**
- `nbpdns_netbox_webhooks_total{result}`;
- `nbpdns_drift_zone_refreshes_total{outcome}`;
- `nbpdns_drift_zone_refresh_duration_seconds`;
- `nbpdns_drift_pending_zones`;
- a `webhooks` section on `/status` and `/api/status`.

**Testing:**
- CI replays real NetBox 4.7 payloads, signed, against `serve` on the lab,
  so CI's memory doesn't grow;
- the lab gains a compose profile, `webhooks`, with NetBox's worker, for
  real end-to-end runs locally, `make test-webhooks`, outside `make ci`.

**Setup:** the user creates the webhook and event rule in NetBox, from a
how-to, by its UI, API or Terraform provider.

### Consequences

- Good: drift shows within seconds of a change in NetBox, at the cost of
  reading only the zones it touched.
- Good: an unsigned or forged event changes nothing, and nbpdns still only
  reads from NetBox.
- Good: a lost webhook costs at most `drift.interval`.
- Bad: the endpoint holds a secret, shared with NetBox.
- Bad: CI's test proves nbpdns's side against recorded payloads. NetBox's
  own sending is proven only locally, with the worker profile.
- Bad: the kept state is now changed per zone as well as per refresh, which
  the code must merge without breaking the reports' immutability.

### Confirmation

- Table tests map every captured event to its zones, and check the
  signature.
- Tests show a burst making one zone refresh, past 100 zones a full one, and
  zone and scheduled refreshes never overlapping.
- An integration test replays a signed event on the lab, and sees the zone's
  drift change within seconds.
- `make test-webhooks` passes with real deliveries.

## Pros and cons of the options

### A full refresh, sooner

- Good: no per-zone merging.
- Bad: a minute of reads at 1,000 zones for each burst, against NetBox and
  every primary.

### Reading NetBox's changelog

- Good: the change IDs, and before and after values, in the trace.
- Bad: a NetBox call per request, and the read-only token needs the
  changelog. Its value comes with writes, in M13.

### A worker in CI's lab

- Good: real deliveries in every pipeline.
- Bad: about 260 MB more, on runners that already kill the integration job.

### nbpdns registering its webhook

- Good: no setup in NetBox.
- Bad: a token that can write NetBox's webhooks and event rules, against
  ADR-0023's read-only token.

## More information

- ADR-0023 (the NetBox client), ADR-0029 (the service), ADR-0033 (the API).
- Recorded in M07's design, 2026-10-08. Implemented by ITEM-0075 to
  ITEM-0080.
