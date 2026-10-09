---
id: M17
title: SIEM export
status: planned # planned | in-progress | done
started:
closed:
---

# M17: SIEM export

## Goal

Every audit event leaves the system reliably and can be shown to be untampered.

## Scope (provisional)

- Sinks: stdout JSON, file, syslog (RFC 5424 over TLS), HTTP (HEC-compatible), OTLP logs (Q-033)
- The wire format (Q-034)
- A transactional outbox with backlog alerting, exporting M08's hash-chained events with their hashes, so that the SIEM can check the chain too (ADR-0039)

## Design, non-goals and acceptance criteria

To be written in this milestone's plan-mode session, before implementation
starts. The milestone's branch is `m17-siem-export` (ADR-0010).
