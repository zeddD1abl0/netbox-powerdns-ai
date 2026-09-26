---
id: ITEM-0021
title: Logging and tracing foundation
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M01
requirements: [REQ-012]
depends_on: [ITEM-0020]
created: 2026-09-27
closed:
---

# ITEM-0021: Logging and tracing foundation

## Goal

Structured logging with `log/slog`, and OpenTelemetry trace context. Every
log line carries `trace_id` and `request_id`, and outbound NetBox calls carry
W3C `traceparent`.

## Acceptance criteria

- [ ] `log.level` and `log.format` (`json` or `text`) are config keys. Logs go to stderr.
- [ ] A handler adds `trace_id`, `span_id` and `request_id` from the context. Each command runs in a root span, and its `request_id` is the invocation's ID.
- [ ] An HTTP transport injects `traceparent` through the OTel propagator. There's no exporter until M04.
- [ ] Tests show every log line has both IDs, and that secrets are redacted.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-09-27: Created from M01's approved design, before implementation
  started.
