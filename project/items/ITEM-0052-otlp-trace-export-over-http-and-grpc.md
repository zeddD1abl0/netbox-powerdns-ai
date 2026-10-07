---
id: ITEM-0052
title: OTLP trace export over HTTP and gRPC
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M04
requirements: [REQ-012]
depends_on: [ITEM-0050]
created: 2026-10-07
closed: 2026-10-07
---

# ITEM-0052: OTLP trace export over HTTP and gRPC

## Goal

Export spans over OTLP, by HTTP/protobuf or gRPC, from every command, when
`tracing.otlp.endpoint` is set (ADR-0029), with headers, a CA file, a
timeout, and the flush when the command ends.

## Acceptance criteria

- [x] The OTLP keys load and validate, `otlp.headers` is a secret with `_FILE`, and an `http://` endpoint warns.
- [x] Tests export over both protocols to in-process receivers, and check the spans, the headers or metadata, TLS with the CA file, and the flush.
- [x] The binary size the exporters add is measured and recorded here.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-07: Created from M04's approved design.
- 2026-10-07: Done.
  - **Keys:** `otlp.endpoint`, `otlp.protocol` (`http/protobuf`, the
    default, or `grpc`), `otlp.headers` (a secret, with `_FILE`),
    `otlp.ca_file` and `otlp.timeout` (10s). The plan named them
    `tracing.otlp.*`, but every key has two levels, which
    `TestKeysAreWellFormed` checks and the reference's example config file
    relies on; M04's file records the change.
  - **`internal/otlp.NewExporter`** builds the exporter, or none without an
    endpoint. For HTTP it adds `/v1/traces` to the endpoint's path, since
    `WithEndpointURL` uses the path as it is. Both exporters would read
    the `OTEL_EXPORTER_OTLP_` variables for any option not passed, so it
    always passes the endpoint, headers, timeout, compression (gzip), and
    TLS (`httpclient.TLSConfig`, now exported, with the CA file), or
    insecure for `http://`, which warns. Headers are `name=value` pairs,
    with values percent-decoded, as `OTEL_EXPORTER_OTLP_HEADERS` is written;
    an error names the pair by number, never its content. It's its own
    package because `internal/httpclient` imports `internal/tracing`.
  - **The session** adds a batching processor when there's an exporter,
    routes the batcher's export errors, which it reports to OpenTelemetry's
    global error handler rather than to its caller, to a warning in the log,
    and shuts the provider down within `otlp.timeout` as the command ends,
    after its root span ends.
  - **Tests:** header parsing, with no secret in any error; export over
    HTTP to an `httptest` collector that decodes the protobuf (the path,
    the header, the span, `service.name`, the `http://` warning), and over
    TLS with and without the CA file; export over gRPC to an in-process
    gRPC trace service (metadata, span, resource), and over TLS with and
    without the CA file; no endpoint; a bad header; a missing CA file. In
    `internal/cli`, `config show` with `otlp.endpoint` set sends its root
    span, with the header, before it returns, and shows the header
    redacted.
  - **Dependencies:** `go.opentelemetry.io/otel/exporters/otlp/otlptrace`,
    with `otlptracehttp` and `otlptracegrpc` v1.47.0 (Apache-2.0), which
    move the OpenTelemetry modules from v1.46.0 to v1.47.0. They bring
    `go.opentelemetry.io/proto/otlp` (Apache-2.0), `google.golang.org/grpc`
    and the `genproto` modules (Apache-2.0), `grpc-gateway/v2` (BSD-3-Clause)
    and `cenkalti/backoff/v5` (MIT). `golang.org/x/net/http/httpguts` checks
    header names. All the licenses are on the allowed list.
  - **Binary size:** `bin/nbpdns` grows from 15,339,563 to 26,338,560 bytes
    (+11.0 MB). Throwaway programs show where it comes from: a bare tracer
    provider builds to 7,760,746 bytes, adding the HTTP exporter to
    20,929,217, and adding the gRPC exporter too to 21,259,592. So the
    protobuf and gRPC code that the HTTP exporter already needs is almost
    all of it, and gRPC itself adds about 0.33 MB.
