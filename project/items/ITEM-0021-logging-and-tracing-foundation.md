---
id: ITEM-0021
title: Logging and tracing foundation
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M01
requirements: [REQ-012]
depends_on: [ITEM-0020]
created: 2026-09-27
closed: 2026-09-29
---

# ITEM-0021: Logging and tracing foundation

## Goal

Structured logging with `log/slog`, and OpenTelemetry trace context. Every
log line carries `trace_id` and `request_id`, and outbound NetBox calls carry
W3C `traceparent`.

## Acceptance criteria

- [x] `log.level` and `log.format` (`json` or `text`) are config keys. Logs go to stderr.
- [x] A handler adds `trace_id`, `span_id` and `request_id` from the context. Each command runs in a root span, and its `request_id` is the invocation's ID.
- [x] An HTTP transport injects `traceparent` through the OTel propagator. There's no exporter until M04.
- [x] Tests show every log line has both IDs, and that secrets are redacted.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-09-27: Created from M01's approved design, before implementation
  started.
- 2026-09-29: Done.
  - **Logging** (`internal/logging`): a `log/slog` JSON or text handler that
    adds `trace_id`, `span_id` and `request_id` from the context.
    - The IDs stay at the top level even inside a `WithGroup`, where slog
      would otherwise nest them. The handler replays its groups over a base
      that already has the IDs.
    - sloglint enforces context-passing calls, no global logger, snake_case
      keys, and nobody else setting the ID keys.
  - **Tracing** (`internal/tracing`):
    - an SDK tracer provider with no exporter, whose sampler and resource
      are set in code, so `OTEL_TRACES_SAMPLER` and `OTEL_SERVICE_NAME`
      change nothing (tested);
    - a `Transport` that runs each request in a client span and injects
      `traceparent` through `propagation.TraceContext`, without otelhttp.
  - **Commands:** every command that loads the configuration runs in
    `app.run`: a root span named after the command, and a per-run
    `request_id` from `crypto/rand.Text`. It logs "command started" and
    "command finished" at debug. `config show` is the first; the `netbox`
    commands follow in ITEM-0024.
  - **Tests:**
    - every line, in both formats, carries the IDs, inside groups too;
    - the Transport's `traceparent` names its client span, a child of the
      command's span (against a local test server);
    - a real command run at debug has the same IDs on every line;
    - secrets are redacted.
  - **Modules added,** as OpenTelemetry is the standard in `CLAUDE.md`,
    with licenses read from each module's LICENSE file:
    - `go.opentelemetry.io/otel`, `otel/trace`, `otel/metric`, `otel/sdk`
      v1.46.0: Apache-2.0
    - `go.opentelemetry.io/auto/sdk` v1.2.1: Apache-2.0
    - `github.com/go-logr/logr` v1.4.4 and `github.com/go-logr/stdr`
      v1.2.2: Apache-2.0
    - `github.com/google/uuid` v1.6.0: BSD-3-Clause
    - `github.com/cespare/xxhash/v2` v2.3.0: MIT

    govulncheck finds nothing. The static binary grew from about 9 MB to
    10 MB.
