---
id: ITEM-0052
title: OTLP trace export over HTTP and gRPC
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M04
requirements: [REQ-012]
depends_on: [ITEM-0050]
created: 2026-10-07
closed:
---

# ITEM-0052: OTLP trace export over HTTP and gRPC

## Goal

Export spans over OTLP, by HTTP/protobuf or gRPC, from every command, when
`tracing.otlp.endpoint` is set (ADR-0029), with headers, a CA file, a
timeout, and the flush when the command ends.

## Acceptance criteria

- [ ] The `tracing.otlp.*` keys load and validate, `tracing.otlp.headers` is a secret with `_FILE`, and an `http://` endpoint warns.
- [ ] Tests export over both protocols to in-process receivers, and check the spans, the headers or metadata, TLS with the CA file, and the flush.
- [ ] The binary size the exporters add is measured and recorded here.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-07: Created from M04's approved design.
