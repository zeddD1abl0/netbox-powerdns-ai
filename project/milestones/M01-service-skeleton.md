---
id: M01
title: Service skeleton
status: planned # planned | in-progress | done
started:
closed:
---

# M01: Service skeleton

## Goal

A runnable `nbpdns` binary and container image with every cross-cutting foundation in place, and no DNS features yet. Each later milestone builds on this without rework.

## Scope (provisional)

- Config registry with the `NBPDNS_` prefix: file, env and flags, plus runtime settings in the DB and generated reference docs (ADR-0005)
- Structured logging (`log/slog`), Prometheus metrics, health and readiness endpoints, OpenTelemetry tracing
- PostgreSQL and SQLite with one migration set, and leader election for the sync worker (ADR-0009)
- OpenAPI pipeline: `api/openapi.yaml`, code generation, contract tests, `make api-lint`, and the reference served by the binary (ADR-0012)
- Audit core: the event model, the registry and persistence. SIEM export is M05.
- Static binary and distroless image, built by GoReleaser
- Container lab (`deploy/dev/`): NetBox with the DNS plugin, and PowerDNS primaries and secondaries in two server groups (REQ-036)

## Design, non-goals and acceptance criteria

To be written in this milestone's plan-mode session, before implementation
starts. The milestone's branch is `m01-service-skeleton` (ADR-0010).
