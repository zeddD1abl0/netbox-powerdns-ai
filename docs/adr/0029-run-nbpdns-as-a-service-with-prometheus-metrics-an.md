---
title: "0029: Run nbpdns as a service with Prometheus metrics and OTLP traces"
status: accepted
date: 2026-10-07
decision-makers: [jordan]
requirements: [REQ-003, REQ-012, REQ-014, REQ-044]
questions: [Q-038]
supersedes:
---

# 0029: Run nbpdns as a service with Prometheus metrics and OTLP traces

## Context and problem statement

`nbpdns drift` (ADR-0027) reports drift once, when someone runs it. M04
makes nbpdns run continuously: `nbpdns serve` refreshes the drift report on
a schedule, so drift can be graphed and alerted on. That needs answers to:
- which metrics nbpdns exposes, and how (Q-038);
- how traces leave the process: M01's tracing (`internal/tracing`) gives
  spans real IDs and sends `traceparent` (ADR-0023), but exports nothing;
- what readiness and liveness mean for a service whose sources, NetBox and the primaries, can fail;
- what the service shows when a refresh fails (Q-027, for the service);
- what the service's listener serves, and to whom, before authentication
  (M10).

## Decision drivers

- Prometheus naming and practice, and OpenTelemetry for traces (CLAUDE.md).
- Metrics are declared once in code, and their reference is generated
  (principle 2).
- A failing upstream must stay visible: it must not hide the metrics that
  show it.
- Bounded cardinality at the scale target, 1,000 zones (REQ-043).
- Nothing that changes state is served before authentication (M10).

## Considered options

1. **Metrics library:** `prometheus/client_golang`; the OpenTelemetry
   metrics SDK with its Prometheus exporter.
2. **Trace export:** OTLP/HTTP only; OTLP/HTTP and OTLP/gRPC.
3. **Readiness:** after the first refresh; while NetBox is readable; as
   soon as the listener is up.
4. **Drift metrics' detail:** per group; per group plus each drifted zone;
   every zone.

## Decision outcome

The user chose the readiness rule, the metrics' detail, and the trace
protocols on 2026-10-07. The status page was the user's addition at the
plan's review.

**Metrics: `github.com/prometheus/client_golang`** (Apache-2.0), the
reference client, which Prometheus's own linting and test helpers come
with. The OpenTelemetry metrics SDK would add a translation layer for the
same output.
- nbpdns exposes only its own metrics. PowerDNS's statistics stay on
  PowerDNS's own `/metrics` (Q-038, REQ-044).
- **Drift, per server group:**
  - zones by state, and RRset changes by kind;
  - problems and warnings;
  - whether the group's primary was read, and its last success.
- **Each drifted zone:** one series, labelled by group, zone and state,
  removed when the zone is back in sync. Series grow only with drift.
- **Refreshes:** counts by outcome, durations, and the last refresh and the
  last complete refresh.
- **Requests** to NetBox and to each primary: counts by status, durations,
  and retries.
- **Build information**, and the Go runtime and process collectors.
- Sync lag and plan size wait for M13's writes, and serial lag on
  secondaries for M14.
- Every metric is declared once in `internal/metrics`. The reference page
  is generated, and a test lints the declarations with promlint.

**Traces: OTLP over HTTP/protobuf, the default, or gRPC**, chosen by
`tracing.otlp.protocol`.
- The user first chose HTTP only. Then they added gRPC, on learning that
  the HTTP exporter requires the gRPC module anyway.
- Every command exports when `tracing.otlp.endpoint` is set, and spans are
  flushed when the command ends.
- Headers, which can hold a collector's token, are a secret with a `_FILE`
  form. A CA file and an `http://` warning work as they do for NetBox and
  PowerDNS.
- The standard `OTEL_` environment variables remain unsupported, as
  `internal/tracing` has said since M01: nbpdns's own keys configure the
  exporter.

**Readiness and liveness:**
- `/readyz` answers 200 once the first refresh has finished, whatever its
  outcome.
- `/livez` answers 200 unless no refresh has started for `drift.interval` +
  `drift.timeout` + 1 minute, which means the refresh loop is stuck.
- Neither looks at NetBox or the primaries. An upstream failure shows in
  the metrics, the status page and the logs; it doesn't make the service
  unready, which could hide its metrics from a scraper.

**Last-known state (Q-027, for the service):**
- A group whose primary can't be read keeps its last successful counts.
  `nbpdns_server_group_up` goes to 0, beside its last-success time.
- If NetBox can't be read, every group keeps its counts, and
  `nbpdns_netbox_up` goes to 0.
- Alert rules use those. The state lives in memory until the database
  (M08) keeps history.

**The status page:** `/status` shows the service's state:
- the schedule;
- the outcome of each refresh;
- NetBox's and each group's state, with the names of drifted zones;
- the trace export.

It's human-readable text by default, and JSON with `?json=1`. It's an
operational endpoint, like `/metrics`, not the REST API: the drift report's
RRset changes come with M06's API (ADR-0012). Its JSON only gains fields,
never loses them, and M06 adds it to `api/openapi.yaml`.

**The listener:**
- Plain HTTP, unauthenticated, at `server.listen`, serving `/livez`,
  `/readyz`, `/status` and `/metrics` only.
- They show group names, zone names, URLs and the groups' errors, but never
  a token, key or header.
- Until authentication (M10), keep the port on a trusted network.

### Consequences

- Good: drift can be graphed and alerted on, with the zone named in the
  alert, at a cost in series that grows only with drift.
- Good: an upstream outage stays visible, in the metrics, the status page
  and the logs, without a restart loop or an unready pod.
- Good: traces go to any OTLP collector, over either protocol.
- Bad: two exporters, and the gRPC module, add to the binary. ITEM-0052
  measures how much.
- Bad: state is lost on restart until M08. A restart re-reads everything
  on its first refresh.
- Bad: the listener is unauthenticated until M10, so network policy must
  protect it.

### Confirmation

- promlint passes over every declared metric, and the generated reference
  is current (`make generate-check`).
- Unit tests check the schedule, readiness and liveness, last-known state,
  the status page and the export over both protocols. Integration tests run
  `nbpdns serve` against the lab.
- The status page's JSON is checked against its reference page, and holds
  no secret.

## Pros and cons of the options

### The OpenTelemetry metrics SDK

- Good: one telemetry SDK for traces and metrics.
- Bad: Prometheus output goes through an exporter bridge, with its own
  naming rules. promlint and `testutil` come with `client_golang` anyway.

### Readiness while NetBox is readable

- Good: a load balancer stops sending to an instance that can't see its
  source of truth.
- Bad: nothing is load-balanced to nbpdns yet, and an unready pod can drop
  out of the scraper's targets, hiding the failure's own metrics.

### Every zone as a series

- Good: the most detail, for dashboards per zone.
- Bad: about 6,000 series at the scale target, most of them always zero.

## More information

- ADR-0023 (`traceparent` on outgoing requests), ADR-0027 (the drift
  report), ADR-0028 (M04's scope).
- Recorded in M04's design, 2026-10-07. Implemented by ITEM-0027,
  ITEM-0049, ITEM-0051 to ITEM-0055.
