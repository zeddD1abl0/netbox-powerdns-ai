---
id: M04
title: Service
status: planned # planned | in-progress | done
started:
closed:
---

# M04: Service

## Goal

nbpdns runs continuously and keeps its drift report current.

## Scope (provisional)

- `nbpdns serve`, which refreshes drift on a schedule
- Health and readiness endpoints
- Prometheus metrics (Q-038) and OTLP trace export
- The static binary and the container image, built by GoReleaser (Q-025, ADR-0015)

## Design, non-goals and acceptance criteria

To be written in this milestone's plan-mode session, before implementation
starts. The milestone's branch is `m04-service` (ADR-0010).
