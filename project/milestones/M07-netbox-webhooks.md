---
id: M07
title: NetBox webhooks
status: done # planned | in-progress | done
started: 2026-10-08
closed: 2026-10-09
---

# M07: NetBox webhooks

## Goal

A signed NetBox webhook makes `nbpdns serve` compare the zones it names
within seconds, and keep its drift report, metrics and API current between
scheduled refreshes. Each such refresh's trace and logs carry NetBox's
request ID and user. The scheduled full refresh still runs as the safety net.

## Non-goals

- **No writes:** nothing is applied to PowerDNS. That's M13, which this
  trigger will serve.
- **No registration in NetBox:** nbpdns doesn't create its own webhook or
  event rule, and keeps its read-only token.
- **No NetBox changelog reads:** ObjectChange IDs, and before and after
  values, come with M13's traceability.
- **No delivery guarantees:** a lost webhook waits for the next scheduled
  refresh. There's no queue of events across restarts (M08).
- **No HA:** with replicas (M09), a webhook reaches one of them.
- **No replay protection:** a replayed webhook only triggers a read-only
  refresh, which is idempotent. M10 and M14 revisit it.

## Phases

| Phase | Items |
|---|---|
| M7a CI | ITEM-0075 Run the integration tests apart from the unit tests |
| M7b Receiving | ITEM-0076 The webhook endpoint: the signature, events to zones, the secret, the spec operation, and its metrics |
| M7c Refreshing | ITEM-0077 Zone refreshes: `drift.Options.Zones`, the service's queue and quiet spell, merging into the state, and tracing |
| | ITEM-0078 Webhooks on `/status` and `/api/status` |
| M7d Testing and docs | ITEM-0079 The lab's worker profile, the replayed integration test, and `make test-webhooks` |
| | ITEM-0080 The webhook docs and the CHANGELOG |

## Acceptance criteria

- [x] ADR-0035 is accepted. REQ-048 exists, Q-037 and Q-054's trigger part
  are answered, and Q-057 holds the import part.
- [x] CI's integration job runs only the integration-tagged packages, and
  only after `unit-test`, on both forges, and `make project-lint` passes.
- [x] `POST /api/netbox-events` accepts only a valid signature, and answers
  as the spec says. Its responses are checked against the spec.
- [x] Every captured NetBox event maps to the right zones, a view to a full
  refresh, and other types are ignored, as table tests show.
- [x] A burst of events makes one zone refresh after the quiet spell, or a
  full refresh past 100 zones. Zone and scheduled refreshes never overlap.
- [x] A zone refresh updates that zone's drift, counts, metrics and records
  in the API, and nothing else.
- [x] Its trace links to the webhook spans, and carries NetBox's request IDs
  and users, as its logs do.
- [x] `/status` and `/api/status` show the webhooks section.
- [x] The replayed integration test passes in CI, and `make test-webhooks`
  passes locally, with real NetBox deliveries.
- [x] The docs above exist, the references are regenerated, and the
  CHANGELOG is updated.
- [x] `/code-review high` and `/security-review` have run. The security
  review runs because M07 adds an authenticated endpoint and a secret.
- [x] The manual verification is recorded. The pipelines pass, and the user
  has merged through an MR with a merge commit: `5bed276`, on 2026-10-09.

## Verification log

Append-only and dated. Record what was run and what was seen.

- 2026-10-08: Every item's commit passed `make check`. `make
  test-integration` passed with `GOFLAGS=-count=1`: internal/cli 43.0 s,
  internal/lab 1.1 s, internal/netbox 6.5 s, internal/powerdns 1.2 s, the
  two tools modules skipped. `make test-webhooks` passed in 4.4 s, with
  NetBox's worker's `send_webhook` jobs in its log.
- 2026-10-08: Manual verification, on the lab started with
  `make lab-up LAB_PROFILES=webhooks`, with `bin/nbpdns serve` on
  `0.0.0.0:8099`, a 64-hex-digit secret in `netbox.webhook_secret_file`,
  `drift.interval: 1h`, Jaeger for its spans, and a view `m07-verify` of
  five zones in sync on lab-a. The webhook and the event rule were made
  with the how-to's REST API commands, as written, with the URL
  `http://host.docker.internal:8099/api/netbox-events`.
  1. Changing z1's www A in NetBox made it drift, with one change, through
     `/api`, 3.8 s after the PATCH. The PATCH sent three events (the record,
     the zone, and its SOA record), all with NetBox's request ID, and they
     made one zone refresh. `/status` showed the webhooks section: the last
     event, `netbox_dns.record updated, request 990697e9-... by admin`,
     nothing waiting, and the last refresh, complete, of
     `m07-verify/z1.m07.example.`. Each event's log line had
     `netbox_request_id` and `netbox_user`, and `zones refreshed` had
     `netbox_request_ids`.
  2. Bulk-creating 150 records across the five zones, in one API call,
     sent 160 events, from 22:09:46 to 22:09:54, and made one zone refresh,
     of the five zones, 3.0 s after the last, which took 0.76 s. No full
     refresh ran. Bulk-creating 120 zones sent 360 events, and made one
     full refresh, `more than 100 zones changed`, which restarted the
     schedule: 120 zones missing, 5 drifted.
  3. In Jaeger, the `drift zone refresh` trace had `FOLLOWS_FROM` links to
     the three webhooks' traces, `netbox.request_ids`, `netbox.users`
     `["admin"]`, `netbox.events` 3, and `nbpdns.zones`. Each
     `POST /api/netbox-events` span had `netbox.request_id`, `netbox.user`,
     `netbox.event` and `netbox.object_type`.
  4. A real event with a signature made with another secret, and one with
     none, were each a 401 problem; `nbpdns_netbox_webhooks_total` counted
     two `bad_signature`, the log had `refused a NetBox webhook whose
     signature didn't verify` with the address, and nothing refreshed.
  The secret was nowhere in the log's 1,093 lines, which had no errors.
  The data, the webhook, the event rule and Jaeger were removed after.
- 2026-10-08: `/code-review high` found nine issues. Six are fixed in
  ITEM-0082: NetBox is asked for 20 zone names a list, a zone refresh no
  longer moves a group's `last_success`, a view's event needs a served
  view, `/status` lists the zones that a waiting full refresh covers, and
  the reference generator handles any security scheme. Three are declined,
  with reasons in ITEM-0082: retrying failed zone refreshes, merging
  outside the lock (3.8 ms at 20,000 zones), and limiting the refusal
  warning. `make check`, `make test-integration` and `make test-webhooks`
  passed again after the fixes.
- 2026-10-08: `/security-review` ran, since M07 adds an authenticated
  endpoint and a secret. No findings. It checked that no request reaches
  the handler without the signature check (the route's pattern, other
  methods, trailing slashes, redirects and escaped paths), the HMAC's
  decoding, length check and constant-time compare, that the secret and
  the signature reach no log, span, status or error, and that event data
  can't add parameters to NetBox's or PowerDNS's requests.
- 2026-10-09: The user pushed the branch at `0a78faa`, and reported its
  pipeline passing without a retry. GitLab pipeline 786 passed, every job
  first time: `unit-test` 616.5 s, then `integration-test`, which started a
  second after it ended, 1,409.9 s; 51 m 52 s in all. GitHub Actions run
  37778553208 passed on its first attempt, `integration-test` starting
  after `unit-test`. Before ITEM-0075, the integration job failed on its
  first try in four of the last five GitLab pipelines.
- 2026-10-09: The user accepted ADR-0036, which restates ADR-0035 as built
  and supersedes it: a moved zone or record makes a full refresh, a view's
  event needs a served view, a zone refresh doesn't move `last_success`,
  Terraform manages only the event rule, and zones are listed by name
  whatever its case, 20 names a request. The code, the explanations and
  REQ-048 cite it. M07 is closed; its last criterion waits for the merge.
- 2026-10-09: **Merged.** The user merged `m07-netbox-webhooks` through
  GitLab merge request !8. `main` is at the merge commit `5bed276`, whose
  parents are the old `main`, `c07c0cc`, and the branch tip, `773296b`, so
  every per-item commit is kept (ADR-0018). `main`'s pipelines passed:
  GitLab 791 and GitHub Actions run 37872245134. The user tagged the merge
  commit `v1.0.0-m07`, an annotated tag, the first milestone tag; its
  pipelines, GitLab 792 and GitHub run 37878653346, were running when this
  was recorded. The user confirmed that only Maintainers can push or merge
  `main` and `v*` branches, or create `v*` tags, generic `v*` packages, and
  the image's `v.*` and `latest` tags.

## Approved design

The plan approved on 2026-10-08, copied verbatim. Its headings are demoted two
levels to nest under this section; the text is unchanged. It's a snapshot.
Where it disagrees with an ADR or `CLAUDE.md`, they win.

### M07: NetBox webhooks

#### Context

`nbpdns serve` refreshes the drift report every `drift.interval`, 5 minutes by
default, so a change in NetBox shows as drift only after the next refresh.
At 1,000 zones that refresh takes about a minute of reads. M07 makes nbpdns
react to NetBox as it changes. A NetBox event-rule webhook, signed with a
shared secret, names the zones a change touched, and nbpdns compares just
those zones within seconds. The scheduled full refresh stays as the safety
net, since a webhook can be lost. This is the trigger M13's writes will use
to keep PowerDNS in step.

Researched in the lab's NetBox 4.7 before the design (2026-10-08):
- **The payload:** a webhook is JSON with `event` (`created`, `updated` or
  `deleted`), `object_type` (such as `netbox_dns.record`), `timestamp`,
  `request` (NetBox's request `id`, `path`, `method` and `user`), `data` (the
  object, as NetBox's API serializes it) and `snapshots` (`prechange` and
  `postchange`).
- **A record event names its zone and view:** `data.zone.name` and
  `data.zone.view.name`. A zone event has `data.name` and `data.view.name`.
- **The signature:** `X-Hook-Signature` is the hex HMAC-SHA512 of the raw
  body, keyed by the webhook's secret, which was checked against a real
  delivery. NetBox sends no `traceparent`.
- **Bursts:** one API call sends several events, all with the same
  `request.id`. Creating two records also updated the zone (its SOA serial)
  and the plugin's own SOA and NS records. A bulk edit sends thousands.
- **Event rules can target the plugin's models:** `netbox_dns.zone`,
  `netbox_dns.record` and `netbox_dns.view`.
- **Delivery needs NetBox's worker** (`manage.py rqworker`), which the lab
  doesn't run. A worker costs about 260 MB.

Decided with the user in the M07 design session, 2026-10-08:
- **What a webhook refreshes (Q-054, its trigger part):** only the affected
  zones, after a short quiet spell that gathers a burst. A view's own change
  refreshes everything. The scheduled full refresh stays.
- **Traceability (Q-037):** NetBox's request ID and user, in the refresh's
  trace, logs and status. The rest of Q-037's chain (apply, each server,
  verification) and NetBox's change IDs come with writes, in M13.
- **Tests:** CI replays real NetBox 4.7 payloads, signed, against nbpdns, so
  CI's memory doesn't grow. The lab gains an optional NetBox worker, a
  compose profile, for real end-to-end runs locally and in the manual
  verification.
- **Setup:** the user configures NetBox's webhook and event rule, from a
  how-to: by its UI, its API, or its Terraform provider. nbpdns keeps its
  read-only token.
- **CI (the user's suggestion):** the integration job runs apart from the
  unit tests, so the two don't compete for the runner's memory.

#### Goal

A signed NetBox webhook makes `nbpdns serve` compare the zones it names
within seconds, and keep its drift report, metrics and API current between
scheduled refreshes. Each such refresh's trace and logs carry NetBox's
request ID and user. The scheduled full refresh still runs as the safety net.

#### Non-goals

- **No writes:** nothing is applied to PowerDNS. That's M13, which this
  trigger will serve.
- **No registration in NetBox:** nbpdns doesn't create its own webhook or
  event rule, and keeps its read-only token.
- **No NetBox changelog reads:** ObjectChange IDs, and before and after
  values, come with M13's traceability.
- **No delivery guarantees:** a lost webhook waits for the next scheduled
  refresh. There's no queue of events across restarts (M08).
- **No HA:** with replicas (M09), a webhook reaches one of them.
- **No replay protection:** a replayed webhook only triggers a read-only
  refresh, which is idempotent. M10 and M14 revisit it.

#### Decisions

##### ADR-0035: refresh the zones that NetBox's webhooks name

- **The endpoint:** `POST /api/netbox-events`, in `api/openapi.yaml`, under
  the API's middleware: flow IDs, spans, problems and metrics.
  - It's off unless `netbox.webhook_secret` is set (a secret, with `_FILE`).
    Until then it's a 404 problem that says how to turn it on.
  - Every request needs a valid `X-Hook-Signature`, the HMAC-SHA512 of the
    raw body, compared in constant time. If it's missing or wrong, the
    request is a 401 problem, and nothing is read from the body.
  - Bodies are limited to 1 MiB (a 413 problem). Malformed JSON, or an event
    without a type, is a 400 problem.
  - An accepted event gets a 202 with no body, at once: the refresh happens
    later.
  - The spec declares the signature as the operation's own security scheme,
    an API key in the `X-Hook-Signature` header. The API's version becomes
    1.1.0.
- **Mapping events to zones:**
  - a `netbox_dns.record` event names `data.zone`, in its view;
  - a `netbox_dns.zone` event names the zone, in its view, from `data`, and
    from `snapshots.prechange` too, so a renamed or moved zone refreshes both
    its old and its new name;
  - a `netbox_dns.view` event asks for a full refresh;
  - other object types, and zones in views that no group serves, are
    ignored, and counted.
- **Refreshing the zones:**
  - The zones gather until no event has come for `drift.webhook_delay`,
    default 3 s, or 30 s since the first, whichever is sooner.
  - Then the groups that serve each zone's view compare only those zones,
    reading NetBox's records for them too.
  - More than 100 zones, or a view event, makes it a full refresh instead.
  - Zone refreshes and scheduled ones never overlap: they share the
    service's refresh loop. A scheduled refresh that runs covers any zones
    waiting.
- **What a zone refresh keeps:** each zone's entry in its group's report,
  the group's counts, the drifted-zone metrics, and NetBox's records are
  replaced for those zones only. Every kept report is a new copy, never
  changed in place, as now. A zone that neither side has any more leaves
  the report. A group whose primary fails keeps its last report, and is
  marked failed, as in a full refresh.
- **Tracing (Q-037):**
  - each webhook request's span gets `netbox.request_id`, `netbox.user`,
    `netbox.event` and `netbox.object_type`, and its log line the same;
  - the zone refresh is a trace of its own, `drift zone refresh`, linked to
    each webhook span it serves, with the zones and NetBox's request IDs
    and users as attributes;
  - its log lines carry `netbox_request_ids`.
- **Metrics:**
  - `nbpdns_netbox_webhooks_total{result}`: `accepted`, `ignored`,
    `bad_signature` and `invalid`;
  - `nbpdns_drift_zone_refreshes_total{outcome}`, and
    `nbpdns_drift_zone_refresh_duration_seconds`;
  - `nbpdns_drift_pending_zones`, the zones waiting.
- **Status:**
  - `/status` and `/api/status` gain a `webhooks` section: whether
    webhooks are on, the last event received (when, NetBox's request ID and
    user), the pending zones, and the last zone refresh (when, its zones and
    outcome);
  - the fields are added, never changed.

##### Requirements and questions

- **REQ-048:** a NetBox webhook, signed with a shared secret, makes nbpdns
  compare the zones it names within seconds. The scheduled full refresh
  remains the safety net.
- **Q-054**'s trigger part is answered, with REQ-048 and ADR-0035. Its
  import part becomes a new question, Q-057, needed by M15.
- **Q-037** is answered: NetBox's request ID and user, in the trace, logs
  and status, from M07. The rest of the chain, and the change IDs, come
  with writes in M13.

#### Design

##### Code

- **`internal/drift`:** `Options.Zones` compares a set of zones, the CLI's
  `--zone` being a set of one. For each zone, the zone's NetBox listing and
  each primary's are read as `--zone` reads them now, up to
  `drift.group_concurrency` groups at once.
- **`internal/webhook`** (new):
  - checks the signature;
  - decodes NetBox's event into the zones it names, by view, or a request
    for a full refresh;
  - unit-tested on real payloads captured from the lab's NetBox 4.7, kept
    as test data.
- **`internal/service`:**
  - `Service.Notify(zones, full, trace links)` queues zones;
  - the run loop waits on its timer or on the quiet spell, whichever comes
    first, and runs the zone refresh in its own trace;
  - `recordZones` merges the results into the kept state, the metrics and
    NetBox's view;
  - the status gains the `webhooks` section.
- **`internal/api`:** the `receiveNetBoxEvent` handler, through the
  generated strict server, with the raw body for the signature. Status
  fields are added to the spec and the handler.
- **`internal/config`:** `netbox.webhook_secret` and `drift.webhook_delay`.
- **`internal/metrics`:** the four new metrics.
- **CI (ITEM-0075):**
  - `make test-integration` runs only the packages that have
    integration-tagged tests, found by their build tag;
  - both forges' `integration-test` jobs wait for `unit-test`
    (`needs: [unit-test]`), so the two never run at once.

##### The lab

- **A compose profile, `webhooks`,** adds `netbox-worker-47`: the same
  NetBox image, running `manage.py rqworker`, with `extra_hosts:
  host.docker.internal:host-gateway`, so NetBox can reach `serve` on the
  Docker host.
- **`make lab-up LAB_PROFILES=webhooks`** starts it. CI never does.
- A how-to section and a local-only make target, `make test-webhooks`,
  which isn't part of `make ci`, run the real end-to-end test.

##### Tests

- **Unit, table-driven:**
  - signatures: good, bad, missing, and computed over another body;
  - every captured event mapped to its zones: record, zone renamed and
    moved, view, deleted, other types;
  - the quiet spell and the 30 s cap;
  - more than 100 zones, or a view, making a full refresh;
  - merging into the state, with counts, metrics and records, a zone that's
    gone, and a group that fails;
  - the endpoint's 202, 401, 400, 404 (off) and 413, each checked against
    the spec.
- **Integration (CI), on the lab:** `serve` with a secret and an interval of
  an hour, so only webhooks can refresh. The test changes a drift-fixture
  record in NetBox, sends that change's event, signed, built from the
  captured payloads, and sees the zone's drift change through the API within
  seconds. A view event makes a full refresh. A wrong signature changes
  nothing.
- **End-to-end (local, `make test-webhooks`):** with the worker profile, a
  real event rule and webhook in the lab's NetBox send to `serve`, and a
  record change shows as drift without a scheduled refresh.

##### Docs

| Section | Pages |
|---|---|
| How-to | "Refresh drift as NetBox changes": set `netbox.webhook_secret`; create the webhook and event rule in NetBox (UI, API, and a Terraform example); check it in `/status`; troubleshoot a 401 or a 404. |
| Explanation | "How nbpdns runs as a service" gains how webhooks trigger zone refreshes: the quiet spell, the full-refresh cases, the safety net, and what a lost webhook means. |
| Reference | The configuration, metrics and API references, regenerated. "Service endpoints" gains `/api/netbox-events` and the status page's `webhooks` fields. |
| How-to | "Run the development lab" gains the `webhooks` profile. |

The CHANGELOG gets lines under Unreleased.

#### Items and phases

| Phase | Items |
|---|---|
| M7a CI | ITEM-0075 Run the integration tests apart from the unit tests |
| M7b Receiving | ITEM-0076 The webhook endpoint: the signature, events to zones, the secret, the spec operation, and its metrics |
| M7c Refreshing | ITEM-0077 Zone refreshes: `drift.Options.Zones`, the service's queue and quiet spell, merging into the state, and tracing |
| | ITEM-0078 Webhooks on `/status` and `/api/status` |
| M7d Testing and docs | ITEM-0079 The lab's worker profile, the replayed integration test, and `make test-webhooks` |
| | ITEM-0080 The webhook docs and the CHANGELOG |

Once the plan is approved, one commit records the design:
- this plan, verbatim, under **Approved design** in M07's file;
- ADR-0035;
- REQ-048, Q-054's trigger part and Q-037 answered, and Q-057;
- the brief's dated answers;
- M07 marked in progress;
- the items.

Each item is then committed on `m07-netbox-webhooks` as it's done.

#### Acceptance criteria

- [ ] ADR-0035 is accepted. REQ-048 exists, Q-037 and Q-054's trigger part
  are answered, and Q-057 holds the import part.
- [ ] CI's integration job runs only the integration-tagged packages, and
  only after `unit-test`, on both forges, and `make project-lint` passes.
- [ ] `POST /api/netbox-events` accepts only a valid signature, and answers
  as the spec says. Its responses are checked against the spec.
- [ ] Every captured NetBox event maps to the right zones, a view to a full
  refresh, and other types are ignored, as table tests show.
- [ ] A burst of events makes one zone refresh after the quiet spell, or a
  full refresh past 100 zones. Zone and scheduled refreshes never overlap.
- [ ] A zone refresh updates that zone's drift, counts, metrics and records
  in the API, and nothing else.
- [ ] Its trace links to the webhook spans, and carries NetBox's request IDs
  and users, as its logs do.
- [ ] `/status` and `/api/status` show the webhooks section.
- [ ] The replayed integration test passes in CI, and `make test-webhooks`
  passes locally, with real NetBox deliveries.
- [ ] The docs above exist, the references are regenerated, and the
  CHANGELOG is updated.
- [ ] `/code-review high` and `/security-review` have run. The security
  review runs because M07 adds an authenticated endpoint and a secret.
- [ ] The manual verification is recorded. The pipelines pass, and the user
  has merged through an MR with a merge commit.

#### Verification

- `make check` on every commit, and `make test-integration` and
  `make release-check` locally.
- **Manual:**
  1. With `LAB_PROFILES=webhooks`, configure a webhook and an event rule in
     the lab's NetBox, as the how-to says. Run `serve` with a secret and a
     long interval, then:
     - change a record, and see its zone drift on `/api` within seconds;
     - see `/status`'s webhooks section, and the log line with NetBox's
       request ID and user.
  2. Bulk-create 150 records across zones, and see one full refresh, not
     150.
  3. With Jaeger, find the zone refresh's trace, linked to the webhook
     spans.
  4. Send a forged event, and see a 401 and no refresh.
  5. Run the first pipeline on this branch, with the CI change, and record
     both jobs' times and whether the integration job was killed.

#### Your side

- Nothing in NetBox until the how-to exists. Then, in your NetBox, create
  the webhook and event rule it describes, with a secret that matches
  `netbox.webhook_secret`.
- Retry the integration job if the runner kills it, as before; M07's CI
  change aims to stop that.
