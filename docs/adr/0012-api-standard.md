---
title: "0012: API standard — OpenAPI 3.1 spec-first, Zalando guidelines"
status: accepted
date: 2026-09-25
decision-makers: [jordan]
requirements: [REQ-013, REQ-017, REQ-018, REQ-019, REQ-037]
questions: [Q-056]
---

# 0012: API standard — OpenAPI 3.1 spec-first, Zalando guidelines

## Context and problem statement

The brief asks for an API that follows "some sort of documented standard"
(REQ-019), with every endpoint documented (REQ-018). The API will be used by the
web UI, a Terraform/OpenTofu provider and an Ansible collection (REQ-017). IaC
clients need predictable, idempotent, stable resources.

## Decision drivers

- The standard must be published and specific enough to check with a linter.
- It must suit declarative IaC clients.
- The documentation must be generated from the same source as the code, so the
  two can't drift.

## Considered options

1. The Zalando RESTful API Guidelines.
2. Google AIP (resource-oriented, but written for gRPC first).
3. The Microsoft REST API Guidelines.

## Decision outcome

Chosen option: **the Zalando RESTful API Guidelines, followed in full**
(Q-056), applied to a **spec-first OpenAPI 3.1** contract.

**Contract**
- `api/openapi.yaml` is the source of truth.
- Server interfaces and types are generated from it. oapi-codegen is the
  candidate, confirmed in M1.
- Responses are contract-tested against the spec.
- The same spec generates the reference docs, and is served by the binary
  with a vendored viewer.

**Conventions (from Zalando)**
- JSON properties in `snake_case`; path segments in `kebab-case`; plural
  resource names.
- **No version in URL paths** (Zalando rule 115). The API evolves only in
  backward-compatible ways: add optional fields and new resources, and never
  remove or repurpose anything. If a breaking change is ever unavoidable, it
  gets a versioned media type (Zalando's preferred mechanism), not a new path
  prefix.
- Errors use `application/problem+json` (RFC 9457).
- Lists use cursor-based pagination.
- Updates use `ETag` with `If-Match` for optimistic concurrency.
- `POST` accepts an `Idempotency-Key` header, so IaC and automation can retry
  safely.
- Deprecations are announced with `Deprecation` and `Sunset` headers, and in
  the spec.

**Enforcement**
- vacuum lints the spec with a Zalando-derived ruleset vendored in
  `api/ruleset.yaml` (`make api-lint`, part of `make check`).
- Any deviation from Zalando needs a new ADR, plus a matching rule exception
  in the ruleset.

### Consequences

- Good: a published, linted standard, and docs that can't drift from the code.
- Good: the idempotency and concurrency rules map directly onto Terraform's
  plan/apply model.
- Bad: with no path version, compatibility discipline is mandatory. Every API
  change is reviewed as a compatibility change, and a spec diff check against
  the last release should be added before v1.0.
- Bad: some Zalando Spectral rules use JavaScript functions vacuum can't run.
  Those are rewritten or dropped, with a note in the ruleset.

### Confirmation

- `make api-lint` runs in `make check` once `api/openapi.yaml` exists (M1).
- Contract tests validate responses against the spec (M1).

## More information

- Zalando RESTful API Guidelines: <https://opensource.zalando.com/restful-api-guidelines/>.
- RFC 9457: <https://www.rfc-editor.org/rfc/rfc9457>.
