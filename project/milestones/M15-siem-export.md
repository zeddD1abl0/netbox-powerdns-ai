---
id: M15
title: SIEM export
status: planned # planned | in-progress | done
started:
closed:
---

# M15: SIEM export

## Goal

Every audit event leaves the system reliably and can be shown to be untampered.

## Scope (provisional)

- Sinks: stdout JSON, file, syslog (RFC 5424 over TLS), HTTP (HEC-compatible), OTLP logs (Q-033)
- The wire format (Q-034)
- A transactional outbox with backlog alerting, and hash-chain verification (Q-035)
- The generated audit event catalogue

## Design, non-goals and acceptance criteria

To be written in this milestone's plan-mode session, before implementation
starts. The milestone's branch is `m15-siem-export` (ADR-0010).
