---
id: M06
title: REST API
status: planned # planned | in-progress | done
started:
closed:
---

# M06: REST API

## Goal

A documented, standards-based API for the drift data, the base for IaC tools and the web UI.

## Scope (provisional)

- The OpenAPI 3.1 pipeline: `api/openapi.yaml`, generated server code, contract tests, and the reference served at `/api/docs` (ADR-0012)
- Read-only, unauthenticated drift endpoints, for trusted networks only until M10
- What IaC will manage through the API (Q-041)

## Design, non-goals and acceptance criteria

To be written in this milestone's plan-mode session, before implementation
starts. The milestone's branch is `m06-rest-api` (ADR-0010).
