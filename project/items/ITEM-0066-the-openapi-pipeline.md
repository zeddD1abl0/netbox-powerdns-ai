---
id: ITEM-0066
title: The OpenAPI pipeline
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M06
requirements: [REQ-019, REQ-037, REQ-046]
depends_on: []
created: 2026-10-08
closed:
---

# ITEM-0066: The OpenAPI pipeline

## Goal

The pipeline every later API item builds on: `api/openapi.yaml`'s
skeleton, oapi-codegen pinned and run by `make generate`, the generated
`net/http` strict server in `internal/api/gen`, the spec embedded and
served at `/api/openapi.yaml`, and contract tests that validate responses
against the spec with libopenapi-validator (ADR-0033). It proves that
oapi-codegen handles the spec before any resource is built on it.

## Acceptance criteria

- [ ] oapi-codegen is pinned in `tools/oapi-codegen/go.mod`; `make generate` writes `internal/api/gen` and the embedded spec, and `generate-check` fails if either is stale.
- [ ] `api/openapi.yaml` has `info` with `x-api-id`, `x-audience` and `version` 1.0.0, `security: [{}]`, the Problem schema and the page object, and passes `make api-lint`.
- [ ] The ruleset has rules for [215] and [219], and `api/testdata` self-tests them.
- [ ] `/api/openapi.yaml` serves the spec, and a contract-test helper validates any response against it.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M06's approved design.
