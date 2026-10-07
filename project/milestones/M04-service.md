---
id: M04
title: Service
status: in-progress # planned | in-progress | done
started: 2026-10-07
closed:
---

# M04: Service

## Goal

`nbpdns serve` runs continuously and keeps the drift report current. It
refreshes the report on a schedule and keeps each group's last-known state
when a refresh fails. It answers `/livez` and `/readyz`, shows its state on
`/status`, exposes `/metrics`, and exports its traces over OTLP/HTTP or
OTLP/gRPC when configured. It's read-only, as before.

## Non-goals

- **Packaging:** GoReleaser, release binaries and the container image are
  M05.
- **No API:** the full drift report over HTTP, with each RRset's changes,
  is M06's spec-first REST API (ADR-0012). `/status` gives the service's
  state, each group's counts, and the names of drifted zones; for the
  changes themselves, use `nbpdns drift` until M06.
- **No authentication on the listener** until M10. It serves nothing that
  changes state.
- **No persistence:** state is in memory and lost on restart; history comes
  with M08.
- **No webhooks:** NetBox changes trigger refreshes from M07. M04 refreshes
  on its schedule only.
- **No configuration reload:** change the config and restart.
- **Metrics that wait for later work:** sync lag and plan size come with
  M13's writes, and serial lag on secondaries with M14.

## Phases

| Phase | Items |
|---|---|
| M4a Re-plan | ITEM-0050 Split packaging into its own milestone (ADR-0028), with the renumbering |
| M4b Telemetry | ITEM-0051 Metrics registry, request metrics and the generated reference; ITEM-0052 OTLP trace export over HTTP and gRPC; ITEM-0027 Log handler chain per record (existing) |
| M4c Service | ITEM-0049 Compare server groups concurrently (existing); ITEM-0053 `nbpdns serve`: schedule, state, health, drift metrics, shutdown; ITEM-0055 The status page, in human and JSON forms |
| M4d Docs | ITEM-0054 Service docs and CHANGELOG |

## Acceptance criteria

- [ ] ADR-0028 and ADR-0029 are accepted. Q-038 is answered, and REQ-044
  exists.
- [ ] The stubs are renumbered, with M05 Packaging. Live references and
  code text use the new numbers. `make project-lint` passes.
- [ ] `nbpdns serve` refreshes on schedule, never overlapping, and keeps
  each group's last-known state. It answers `/livez` and `/readyz` as
  ADR-0029 says, and shuts down cleanly on SIGTERM.
- [ ] `/metrics` has every metric in the table. The reference is
  generated, and promlint passes.
- [ ] `/status` shows the service's state, as text by default and as JSON
  with `?json=1`. Every JSON field is in the reference, and no secret is in
  either form.
- [ ] With `tracing.otlp.endpoint` set, every command exports its spans
  over the chosen protocol, HTTP or gRPC, with one trace per refresh under
  `serve`. Headers and the CA file work for both.
- [ ] Groups are compared concurrently, and the log handler no longer
  rebuilds per record, with a benchmark.
- [ ] Unit and integration tests cover the above against the lab's NetBox
  4.7 and PowerDNS 5.1. The lab gains no containers.
- [ ] The docs pages above exist, the references are current, and the
  CHANGELOG is updated.
- [ ] `/code-review high` has run. `/security-review` runs, since M04 adds
  a network listener and a secret (the OTLP headers).
- [ ] The manual verification is recorded. The GitLab and GitHub pipelines
  pass, and the user has merged through an MR with a merge commit.

## Decided after approval

> [!IMPORTANT]
> Changed during implementation, with the reasons recorded in the items
> named. These override the approved design below.
>
> - **The OTLP keys are `otlp.endpoint`, `otlp.protocol`, `otlp.headers`,
>   `otlp.ca_file` and `otlp.timeout`**, not `tracing.otlp.*`. Every key
>   has two levels, which the registry checks, and the reference's example
>   config file relies on (ITEM-0052).
> - **The reference page is "Service endpoints"**, `service-endpoints.md`,
>   not "HTTP endpoints": Google's heading rule wants a heading's first word
>   in sentence case, which an acronym can't be (ITEM-0055).

## Verification log

Append-only and dated. Record what was run and what was seen.

## Approved design

The plan approved on 2026-10-07, copied verbatim. Its headings are demoted two
levels to nest under this section; the text is unchanged. It's a snapshot.
Where it disagrees with an ADR or `CLAUDE.md`, they win.

### M04: Service

#### Context

M03's `nbpdns drift` answers "does PowerDNS serve what NetBox says?" once,
when someone runs it. M04 makes nbpdns run continuously. `nbpdns serve`
refreshes the drift report on a schedule. It answers health and readiness
checks, exposes Prometheus metrics, and exports its traces over OTLP, so
drift can be graphed and alerted on.

Decided with the user in the M04 design session, 2026-10-07:
- **Packaging moves out** (ADR-0028). GoReleaser, the binaries and the
  container image become their own milestone, the new M05 "Packaging", and
  the stubs after it shift up one number. M04 is the service only.
- **Readiness:** ready once the first refresh has finished, whatever its
  outcome. Failures of NetBox or a primary show in the metrics and logs.
- **Drift metrics:** counts per server group, plus one series per zone
  that's currently drifted, labelled by group and zone.
- **Traces:** first OTLP/HTTP only. But `otlptracehttp` requires the
  `google.golang.org/grpc` module anyway, so at review the user added
  OTLP/gRPC as well. `tracing.otlp.protocol` chooses between them, with
  `http/protobuf` as the default, as in the OTLP spec. ITEM-0052 records
  the binary size the two exporters add.
- **A status page** (the user, at review): `/status` shows the service's
  state as a human-readable page by default, and as JSON with `?json=1`.

Milestone numbers below are the new ones: M05 Packaging, M06 REST API,
M07 NetBox webhooks, M08 SQLite, M09 PostgreSQL and HA, M10
Authentication, M11 SSO and RBAC, M12 Web UI, M13 Write path, M14 Change
safety, M15 Brownfield import, M16 SIEM export, M17 Production hardening,
M18 Terraform provider, M19 Ansible and v1.0.

#### Goal

`nbpdns serve` runs continuously and keeps the drift report current. It
refreshes the report on a schedule and keeps each group's last-known state
when a refresh fails. It answers `/livez` and `/readyz`, shows its state on
`/status`, exposes `/metrics`, and exports its traces over OTLP/HTTP or
OTLP/gRPC when configured. It's read-only, as before.

#### Non-goals

- **Packaging:** GoReleaser, release binaries and the container image are
  M05.
- **No API:** the full drift report over HTTP, with each RRset's changes,
  is M06's spec-first REST API (ADR-0012). `/status` gives the service's
  state, each group's counts, and the names of drifted zones; for the
  changes themselves, use `nbpdns drift` until M06.
- **No authentication on the listener** until M10. It serves nothing that
  changes state.
- **No persistence:** state is in memory and lost on restart; history comes
  with M08.
- **No webhooks:** NetBox changes trigger refreshes from M07. M04 refreshes
  on its schedule only.
- **No configuration reload:** change the config and restart.
- **Metrics that wait for later work:** sync lag and plan size come with
  M13's writes, and serial lag on secondaries with M14.

#### Decisions

##### ADR-0028: split packaging into its own milestone

It supersedes ADR-0019's milestone table, as ADR-0019's own rule allows for
planned stubs that have no work yet.
- **New M05 "Packaging":** GoReleaser, static linux amd64 and arm64
  binaries, checksums, and the container image (Q-025). The image is
  reviewed against ADR-0015, and where it's published is decided in M05's
  design.
- **Renumbering:** M05 to M18 become M06 to M19. Each stub's file, ID and
  branch name changes.
- **Live references** to the old numbers are updated:
  - open items (ITEM-0040, M12 to M13);
  - the requirements' "needed by" column;
  - the docs;
  - the CHANGELOG and CLAUDE.md;
  - code text, where `enforce (from M12)` becomes `enforce (from M13)`,
    along with the config reference's wording.
- **Records keep their numbers:** done milestones, the brief, accepted
  ADRs, and closed items' notes. ADR-0028 has the old-to-new table.

##### ADR-0029: run nbpdns as a service, with Prometheus metrics and OTLP traces

It answers Q-038, gives Q-027 its service default, and produces REQ-044.
- **Metrics:** `github.com/prometheus/client_golang`, the reference client
  (Apache-2.0). Only nbpdns's own metrics are exposed; PowerDNS's
  statistics stay on PowerDNS's own `/metrics`. Every metric is declared
  once in `internal/metrics`, with a generated reference page (principle
  2), and a test runs promlint over the declarations.
- **Traces:** `otlptracehttp` and `otlptracegrpc`, from
  `go.opentelemetry.io/otel/exporters/otlp/otlptrace` (Apache-2.0), chosen
  by `tracing.otlp.protocol`. Every command exports when
  `tracing.otlp.endpoint` is set. Spans are flushed when the command ends.
- **The status page:** `/status` is human-readable text by default, and
  JSON with `?json=1`. It's an operational endpoint, like `/metrics`, not
  the REST API. Its JSON only gains fields, never loses them. M06 adds it
  to `api/openapi.yaml`.
- **Readiness and liveness:**
  - `/readyz` answers 200 once the first refresh has finished.
  - `/livez` answers 200 unless no refresh has started for
    `drift.interval` + `drift.timeout` + 1 minute, which means the loop is
    stuck.
  - Neither looks at NetBox or the primaries.
- **Last-known state (Q-027, for the service):**
  - A group whose primary can't be read keeps its last successful counts
    in the metrics. `nbpdns_server_group_up` goes to 0, beside the group's
    last-success time.
  - If NetBox can't be read, every group keeps its last counts, and
    `nbpdns_netbox_up` goes to 0.
  - Alerts use those, as the how-to's example rules show.
- **The listener:** plain HTTP, unauthenticated, serving only `/livez`,
  `/readyz`, `/status` and `/metrics`. They carry group names, zone names,
  URLs, and the groups' errors, but no secrets. So the docs say to keep the
  port on a trusted network until M10.

##### Requirements

- REQ-044: "nbpdns exposes Prometheus metrics for its drift reports, its
  refreshes, and its requests to NetBox and PowerDNS. PowerDNS's own
  statistics stay on PowerDNS's `/metrics`."
- Q-038 moves to Answered.
- Q-025's "needed by" becomes M05 (binary, image) and M17 (the rest).

#### Design

##### Configuration (new keys, all in the registry and the generated reference)

| Key | Default | Does |
|---|---|---|
| `server.listen` | `:8080` | The address `nbpdns serve` listens on, for `/livez`, `/readyz`, `/status` and `/metrics` |
| `drift.interval` | `5m` | Time from one refresh's start to the next one's. At least `10s`. |
| `drift.timeout` | `10m` | A refresh's time limit |
| `drift.group_concurrency` | `4` | How many server groups are compared at once (ITEM-0049). `nbpdns drift` uses it too. |
| `tracing.otlp.endpoint` | none | The collector's URL, such as `https://otel.example.com:4318`. For `http/protobuf`, `/v1/traces` is added, as the OTLP spec says. With `http://` the export isn't encrypted, and it warns. Unset, nothing is exported. |
| `tracing.otlp.protocol` | `http/protobuf` | `http/protobuf` or `grpc` |
| `tracing.otlp.headers` | none | A secret: request headers, or gRPC metadata, as `name=value,name=value`, such as an auth token. It also takes `_FILE`. |
| `tracing.otlp.ca_file` | none | A PEM file of CA certificates to trust for the collector |
| `tracing.otlp.timeout` | `10s` | How long one export may take |

- `durationKey` gains a lower bound for `drift.interval`.
- An `http://` OTLP endpoint warns, as the NetBox and PowerDNS URLs do.

##### The service (`internal/service`)

- **Schedule:** `Run(ctx)` refreshes at start, then every `drift.interval`,
  measured from start to start. A refresh that overruns delays the next,
  with a warning, so refreshes never overlap.
- **Each refresh:**
  - It gets its own `request_id`, and a root span, `drift refresh`, so each
    refresh is one trace.
  - It's bounded by `drift.timeout`.
  - It runs exactly what `nbpdns drift` runs. The CLI's code that builds
    the clients and calls `drift.Run` moves into one function that both
    commands call.
- **State:**
  - It keeps the latest report, and each group's last successful result,
    for the metrics.
  - Readiness and the loop's last-start time are guarded by a mutex.
- **Logs:**
  - info: one line per group per refresh, with its status and counts;
  - warn: a failed group, with its reason, and the report's warnings;
  - debug: each drifted zone.
- **Shutdown:** on SIGINT or SIGTERM (`main` already cancels the context),
  it stops scheduling, cancels the refresh in flight, drains the HTTP
  server for up to 10 seconds, and flushes the spans. It exits 0.
- **HTTP server:** the standard library's, with read-header, read, write
  and idle timeouts. `GET` and `HEAD` work on the four paths; anything
  else gets 404 or 405.

##### The status page (`/status`)

- **What it shows:**
  - nbpdns's version, start time and uptime;
  - readiness and liveness;
  - the schedule: interval, timeout, the last refresh's start, end,
    duration and outcome, the last complete refresh, the next refresh due,
    and the counts of refreshes by outcome;
  - NetBox: its URL, whether the last refresh could read it, and its last
    error;
  - each server group:
    - its primary's URL and its status;
    - its last success, and its last error;
    - its zone counts by state;
    - the names and states of its drifted zones;
    - its problem and warning counts;
  - trace export: on or off, with the endpoint and protocol.
  It never shows a token, key, or header.
- **Human form**, the default: `text/plain; charset=utf-8`, with the same
  aligned tables as the CLI's output (`writeTable`). It reads in a browser
  and with `curl`, and needs no assets.
- **JSON** with `?json=1`: `application/json`, with snake_case fields. Its
  fields are documented in the reference page "HTTP endpoints". A test
  decodes that page's example strictly, and checks that every field is
  documented.
- Before the first refresh, both forms say there's no report yet.

##### Metrics (`internal/metrics`; names follow Prometheus practice)

| Metric | Type | Labels |
|---|---|---|
| `nbpdns_drift_refreshes_total` | counter | `outcome`: `complete`, `incomplete`, `failed` |
| `nbpdns_drift_refresh_duration_seconds` | histogram | none |
| `nbpdns_drift_last_refresh_timestamp_seconds` | gauge | none |
| `nbpdns_drift_last_complete_refresh_timestamp_seconds` | gauge | none |
| `nbpdns_drift_zones` | gauge | `group`, `state`: `in_sync`, `drift`, `missing`, `inactive_in_netbox`, `ignored`, `unmanaged` |
| `nbpdns_drift_rrset_changes` | gauge | `group`, `kind`: `missing`, `extra`, `changed` |
| `nbpdns_drift_zone_drifted` | gauge, 1 | `group`, `zone`, `state`. One series per drifted zone, removed when the zone is back in sync. |
| `nbpdns_drift_problems`, `nbpdns_drift_warnings` | gauge | `group` |
| `nbpdns_netbox_up` | gauge | none |
| `nbpdns_server_group_up` | gauge | `group` |
| `nbpdns_server_group_last_success_timestamp_seconds` | gauge | `group` |
| `nbpdns_http_client_requests_total` | counter | `service` (`NetBox` or `PowerDNS`), `target` (`netbox` or the group's name), `method`, `code` (the status, or `error`) |
| `nbpdns_http_client_request_duration_seconds` | histogram | `service`, `target`, `method` |
| `nbpdns_http_client_retries_total` | counter | `service`, `target` |
| `nbpdns_build_info` | gauge, 1 | `version`, `revision`, `goversion` |

- The Go runtime and process collectors are registered too.
- `internal/httpclient.Options` gains an `Observer` interface, which the
  metrics implement. One-shot commands pass none.
- `docs/reference/metrics.md` is generated by `gendocs`.

##### Server groups compared concurrently (ITEM-0049)

`drift.Run` lists, reads and compares groups concurrently, up to
`drift.group_concurrency`. NetBox is still read once. The report keeps the
configuration's group order.

##### Logging (ITEM-0027)

The log handler stops rebuilding its chain for every record of a grouped
logger. That matters now that nbpdns logs continuously.

##### Lab and tests

- **No new containers.**
- **Unit tests:**
  - the schedule, overrun and shutdown, with an in-memory refresh;
  - readiness and liveness transitions;
  - last-known state;
  - the metric values, checked with `testutil`;
  - promlint over every declared metric;
  - the observer's request and retry counts;
  - OTLP export over both protocols, to in-process receivers: an
    `httptest` server that decodes the protobuf, and a gRPC server with
    the OTLP trace service. They check the spans, the headers or metadata,
    TLS with the CA file, and the flush at exit;
  - `/status` in both forms: before the first refresh, after a complete
    one, and with a failed group. The JSON is checked against the
    reference page's example, and holds no secret.
- **Integration tests:**
  - `nbpdns serve` runs in-process against the lab, on the drift fixture,
    with `server.listen` on `127.0.0.1:0` and the bound address read from
    its log;
  - `/readyz` answers 503, then 200;
  - `/metrics` shows each fixture case's counts and drifted-zone series;
  - `/status` lists the fixture's drifted zones, in both forms;
  - a second group whose primary refuses connections has `up` 0, and the
    other is still reported;
  - cancelling the context stops it cleanly.

##### Docs

| Section | Pages |
|---|---|
| Tutorial | "Run nbpdns as a service", in the lab: serve, check readiness, read `/status` and the metrics, change PowerDNS, and watch the next refresh |
| How-to | "Monitor drift with Prometheus": a scrape config, and example alert rules for drift, a group that's down, and a stale refresh. "Export traces to an OpenTelemetry collector", over HTTP or gRPC. |
| Reference | Metrics (new, generated); "HTTP endpoints" (new): `/livez`, `/readyz`, `/status` with its JSON fields, and `/metrics`; configuration and command line (generated) |
| Explanation | "How nbpdns runs as a service": the schedule, last-known state, readiness and liveness, metric design and cardinality, and the listener's exposure |

`CHANGELOG.md` gets lines under Unreleased.

#### Items and phases

| Phase | Items |
|---|---|
| M4a Re-plan | ITEM-0050 Split packaging into its own milestone (ADR-0028), with the renumbering |
| M4b Telemetry | ITEM-0051 Metrics registry, request metrics and the generated reference; ITEM-0052 OTLP trace export over HTTP and gRPC; ITEM-0027 Log handler chain per record (existing) |
| M4c Service | ITEM-0049 Compare server groups concurrently (existing); ITEM-0053 `nbpdns serve`: schedule, state, health, drift metrics, shutdown; ITEM-0055 The status page, in human and JSON forms |
| M4d Docs | ITEM-0054 Service docs and CHANGELOG |

Once the plan is approved, one commit records the design:
- this plan, verbatim, under **Approved design** in M04's file;
- ADR-0028 and ADR-0029;
- REQ-044, and Q-038 answered;
- the brief's dated answers;
- M04 marked in progress;
- the new items.

ITEM-0050's renumbering follows as its own commit. Each item is then
committed as it's done, on `m04-service`.

#### Acceptance criteria

- [ ] ADR-0028 and ADR-0029 are accepted. Q-038 is answered, and REQ-044
  exists.
- [ ] The stubs are renumbered, with M05 Packaging. Live references and
  code text use the new numbers. `make project-lint` passes.
- [ ] `nbpdns serve` refreshes on schedule, never overlapping, and keeps
  each group's last-known state. It answers `/livez` and `/readyz` as
  ADR-0029 says, and shuts down cleanly on SIGTERM.
- [ ] `/metrics` has every metric in the table. The reference is
  generated, and promlint passes.
- [ ] `/status` shows the service's state, as text by default and as JSON
  with `?json=1`. Every JSON field is in the reference, and no secret is in
  either form.
- [ ] With `tracing.otlp.endpoint` set, every command exports its spans
  over the chosen protocol, HTTP or gRPC, with one trace per refresh under
  `serve`. Headers and the CA file work for both.
- [ ] Groups are compared concurrently, and the log handler no longer
  rebuilds per record, with a benchmark.
- [ ] Unit and integration tests cover the above against the lab's NetBox
  4.7 and PowerDNS 5.1. The lab gains no containers.
- [ ] The docs pages above exist, the references are current, and the
  CHANGELOG is updated.
- [ ] `/code-review high` has run. `/security-review` runs, since M04 adds
  a network listener and a secret (the OTLP headers).
- [ ] The manual verification is recorded. The GitLab and GitHub pipelines
  pass, and the user has merged through an MR with a merge commit.

#### Verification

- `make check` on every commit, and `make test-integration` against the lab.
- **Manual, on a fresh lab:**
  1. Follow the tutorial: serve, wait for `/readyz`, read `/status` in a
     browser and with `?json=1`, read `/metrics`, change a record on lab-a,
     and see the next refresh change the gauges, add a drifted-zone series,
     and list the zone on `/status`.
  2. Stop lab-a's PowerDNS. `nbpdns_server_group_up{group="lab-a"}` goes
     to 0, the last counts stay, and `/readyz` stays 200. Start it again,
     and the group recovers.
  3. Use a temporary Prometheus container, not part of the lab, to scrape
     `serve` with the how-to's config, and see the example drift alert
     fire.
  4. Use a temporary Jaeger or OpenTelemetry Collector container to
     receive the traces, over HTTP and then over gRPC. See one trace per
     refresh, with the NetBox and PowerDNS requests as child spans.
  5. Send SIGTERM during a refresh. nbpdns exits 0 within seconds, and
     logs the shutdown.
  6. Scale: serve the 1,000-zone data set from M03. Each refresh's
     duration matches the one-shot run, and resident memory stays flat over
     several refreshes.

#### Your side

- Nothing new for M04. The packaging questions (where images and binaries
  are published, and signing) come in M05's design.
