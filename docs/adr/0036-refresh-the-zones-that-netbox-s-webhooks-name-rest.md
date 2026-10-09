---
title: '0036: Refresh the zones that NetBox''s webhooks name, restated as built'
status: accepted # proposed | accepted | rejected | deprecated | superseded by ADR-NNNN
date: 2026-10-09
decision-makers: [jordan]
requirements: [REQ-044, REQ-046, REQ-048]
questions: [Q-037, Q-054]
supersedes: ADR-0035
---

# 0036: Refresh the zones that NetBox's webhooks name, restated as built

## Context and problem statement

ADR-0035 chose to have NetBox's signed webhooks refresh only the zones they
name, between scheduled full refreshes. Building it in M07 found five places
where its text doesn't hold:

- **Snapshots name related objects by ID.** In NetBox 4.7, an event's `data`
  nests a zone's view, and a record's zone, by name, but
  `snapshots.prechange` has only their IDs. ADR-0035 said a zone event names
  the zone's old view from the snapshot. It can't: the event names where a
  moved zone, or a moved record, was only by an ID that nbpdns doesn't keep.
- **Most view events are about views that nbpdns doesn't serve.** ADR-0035
  made every view event a full refresh. Its code review found that an edit
  to any view, served or not, would then read every zone of every group.
- **A zone refresh compares only some zones.** ADR-0035 didn't say what it
  does to a group's `last_success`. Moving it would make a group whose full
  refreshes keep failing look fresh, though most of its report is old.
- **NetBox's Terraform provider can't set a webhook's secret.**
  `e-breuninger/netbox`'s `netbox_webhook` has no such argument, in its docs
  or its schema, and nbpdns refuses an unsigned webhook. ADR-0035 named
  Terraform as one way to set NetBox up.
- **The DNS plugin keeps a zone's name in the case it was given,** and its
  `name` filter matches exactly. Listing zones by the lowercase names that
  events give would miss `Example.com` (ITEM-0081).

Accepted ADRs aren't edited, so this ADR restates ADR-0035 with these
corrected. **The decision itself is unchanged.**

## Decision drivers

As in ADR-0035:
- The zones a change touches are known from the event. A full refresh for
  each burst would cost a minute of reads at scale.
- A webhook can be lost, so the scheduled refresh must stay.
- The endpoint must refuse anything NetBox didn't sign, while the rest of
  the API has no authentication (ADR-0033).
- nbpdns keeps its read-only NetBox token (ADR-0023).
- CI's runners are short of memory.

And, from building it:
- Only what an event says by name can be refreshed by name.
- The status and metrics that alerts read must not look fresher than the
  report is.

## Considered options

ADR-0035's options and choices stand: only the zones a webhook names; NetBox's
request ID and user for traceability; replayed payloads in CI, with NetBox's
worker on demand in the lab; and setup by the user, from nbpdns's docs.

For a moved zone or record, the options were:
1. **A full refresh**, which covers the old place too.
2. Keeping each zone's and view's NetBox ID, to name the old place.
3. Reading the old object from NetBox, by its ID.

## Decision outcome

Chosen option for a moved zone or record: **a full refresh**. Moves are rare,
and the other options would keep or read NetBox's IDs for one case. The rest
is ADR-0035's decision, restated with the corrections marked.

**The endpoint:** `POST /api/netbox-events`, in the OpenAPI document, under
the API's middleware.
- **Off by default:** it's off unless `netbox.webhook_secret` is set, a
  secret with a `_FILE` form, of at least 16 characters. Until then it's a
  404 problem saying how to turn it on.
- **Signed:** every request needs a valid `X-Hook-Signature`, the hex
  HMAC-SHA512 of the raw body, compared in constant time. Without one, it's
  a 401 problem, and nothing is read from the body. The check runs before
  the generated server decodes the body, on the routes whose `security` in
  the spec names the signature's scheme.
- **Bounded:** bodies are limited to 1 MiB (a 413 problem). A malformed
  event is a 400.
- **Fast:** an accepted event gets a 202, at once.
- **In the spec:** the signature is the operation's own security scheme. The
  API's version becomes 1.1.0.

**Events to zones:**
- a `netbox_dns.record` event names its zone, in the zone's view, unless
  the record moved to another zone: then it asks for a full refresh
  (**corrected**);
- a `netbox_dns.zone` event names the zone, and its old name, from
  `snapshots.prechange`, if the change renamed it, in its view; a zone that
  moved to another view asks for a full refresh (**corrected**);
- a `netbox_dns.view` event asks for a full refresh, if a server group
  serves the view, by its name or its old one; otherwise it's ignored
  (**corrected**);
- other types, and zones in views that no group serves, are ignored and
  counted.

**Zone refreshes:**
- Zones gather until no event has come for `drift.webhook_delay`, from
  100 ms to 30 s, default 3 s, or for 30 s since the first, whichever is
  sooner.
- Then each group that serves a zone's view compares those zones' names, in
  every view it serves, as a full refresh would. NetBox is listed by name,
  whatever its case, 20 names a request, so that the URL stays within what
  proxies take; each primary is listed once (**corrected**).
- More than 100 zones, or a full refresh that an event asked for, makes a
  full refresh instead, which starts the schedule again.
- Zone and scheduled refreshes share the service's refresh loop, so they
  never overlap. A scheduled refresh covers any zones waiting.
- A zone refresh replaces those zones' entries in each group's report, the
  group's counts, the drifted-zone metrics, and NetBox's records. Every kept
  report is a new copy. A zone that neither side has any more leaves the
  report. A group whose primary fails keeps its last report, and is marked
  failed. The zones of a zone refresh that fails wait for the next scheduled
  refresh.
- A group's `last_success`, its metric, and the `as_of` of NetBox's
  records stay those of the last full refresh: zones that webhooks named may
  be newer (**corrected**).

**Traceability (Q-037), now:** each webhook's span and log line carry
NetBox's request ID, user, event and object type; each zone refresh is a
trace of its own, linked to the webhook spans it serves, with the zones,
request IDs and users as attributes and in its logs. The rest of Q-037's
chain, and NetBox's change IDs, come with writes in M13.

**Metrics and status:** `nbpdns_netbox_webhooks_total{result}`,
`nbpdns_drift_zone_refreshes_total{outcome}`,
`nbpdns_drift_zone_refresh_duration_seconds` and
`nbpdns_drift_pending_zones`, and a `webhooks` section on `/status` and
`/api/status`.

**Testing:** CI replays real NetBox 4.7 payloads, signed, against `serve` on
the lab; the lab's compose profile `webhooks` adds NetBox's worker, for
`make test-webhooks`, outside `make ci`.

**Setup:** the user creates the webhook and the event rule in NetBox, from a
how-to, by its web interface or its REST API. Terraform can manage the event
rule, given the webhook's ID, but not the webhook, whose secret its NetBox
provider can't set (**corrected**).

### Consequences

As in ADR-0035:
- Good: drift shows within seconds of a change in NetBox, at the cost of
  reading only the zones it touched.
- Good: an unsigned or forged event changes nothing, and nbpdns still only
  reads from NetBox.
- Good: a lost webhook costs at most `drift.interval`.
- Bad: the endpoint holds a secret, shared with NetBox.
- Bad: CI's test proves nbpdns's side against recorded payloads. NetBox's
  own sending is proven only locally, with the worker profile.
- Bad: the kept state is now changed per zone as well as per refresh.

And:
- Good: an edit to a view that nbpdns doesn't serve costs nothing.
- Good: a group's `last_success` still says when its whole report was last
  compared, so a stale group's alerts fire.
- Bad: moving a zone or a record reads everything.
- Bad: `last_success` and `as_of` can be older than some of the zones they
  date.
- Bad: a webhook can't be set up entirely in Terraform.

### Confirmation

- Table tests map every captured NetBox 4.7 event, kept as test data with
  the signatures NetBox sent, to its zones, its views, or nothing.
- A property test changes both sides, and checks that a zone refresh merged
  into the last full report equals a new full report.
- Tests show a burst making one zone refresh, past 100 zones a full one,
  zone and scheduled refreshes never overlapping, and a zone refresh
  leaving `last_success` as it was.
- The integration test replays a signed event on the lab, and sees the
  zone's drift change within seconds; `make test-webhooks` passes with
  NetBox's own deliveries.

## Pros and cons of the options

### A full refresh for a moved zone or record

- Good: correct with what the event says, and no state to keep.
- Bad: a minute of reads at 1,000 zones, for a rare change.

### Keeping NetBox's IDs

- Good: a move refreshes only the two places.
- Bad: a map of every zone's and view's ID, kept in step with NetBox, for a
  rare case.

### Reading the old object from NetBox

- Good: no state.
- Bad: the old object is gone once it moved: its ID now names the new
  place, so the read says nothing about the old one.

## More information

- ADR-0035, which this restates. ADR-0023 (the NetBox client), ADR-0029 (the
  service), ADR-0033 (the API).
- Found while building M07: ITEM-0076 (the snapshots), ITEM-0081 (the case
  of names), ITEM-0080 (the Terraform provider), ITEM-0082 (the code
  review's findings).
