---
title: "0033: A read-only API, spec-first, generated with oapi-codegen"
status: accepted
date: 2026-10-08
decision-makers: [jordan]
requirements: [REQ-013, REQ-017, REQ-018, REQ-019, REQ-037, REQ-046, REQ-047]
questions: [Q-041]
supersedes:
---

# 0033: A read-only API, spec-first, generated with oapi-codegen

## Context and problem statement

Until M06, nbpdns's state can be read only as `/status` and the Prometheus
metrics (ADR-0029), which were built for people and for Prometheus, not as
a contract. The web UI (M12), the Terraform/OpenTofu provider (M18) and the
Ansible collection (M19) need an API. ADR-0012 sets its standard: OpenAPI
3.1, spec-first, under the Zalando guidelines in full, with oapi-codegen
as the candidate generator.

Q-041 asked what IaC manages. On 2026-10-08, the user answered: nbpdns's
own configuration, and reading DNS records, so that Terraform can read a
zone's records from nbpdns and push them to other providers. Records are
still written only in NetBox (ADR-0004).

M06 decides what the first API serves, and how it's built. Authentication
arrives in M10.

## Decision drivers

- One source: the code, the docs and the tests all come from
  `api/openapi.yaml` (ADR-0012).
- Every response is checked against the spec.
- Zalando in full, with any deviation recorded (ADR-0012).
- Stable IDs that IaC can import later: a group's name, a zone's name.
- Read-only until authentication exists: nothing that changes state.
- Few dependencies, each established and licensed MIT, BSD, Apache-2.0 or
  MPL-2.0.

## Considered options

1. **Generator:** oapi-codegen's `net/http` strict server; ogen;
   hand-written handlers, checked only by contract tests.
2. **Contract tests:** pb33f's libopenapi-validator; kin-openapi's
   `openapi3filter`.
3. **Records:** NetBox's side only; both NetBox's and PowerDNS's; none
   yet.
4. **Before authentication:** a temporary deviation from Zalando's [104];
   no API until M10.

## Decision outcome

The user chose the resources, the records and the scope on 2026-10-08.
The tools follow from the drivers.

**The contract:**
- `api/openapi.yaml`, OpenAPI 3.1, is the source of truth.
- **oapi-codegen** v2.8 or later (Apache-2.0) generates `net/http`
  strict-server interfaces and types into `internal/api/gen`. It's pinned
  in `tools/oapi-codegen/go.mod`. Its runtime,
  `github.com/oapi-codegen/runtime` (Apache-2.0), binds the parameters.
- `make generate` regenerates the code, and an embedded copy of the spec
  that the binary serves. `make generate-check` fails if either is stale.
- **Contract tests** check every response, errors included, against the
  spec, with **libopenapi-validator** (MIT), which supports OpenAPI 3.0 to
  3.2.
- oapi-codegen calls its 3.1 support "initial", so the spec keeps to plain
  3.1: `type: [T, "null"]` for nullable fields, and no webhooks. If it
  can't generate from the spec, the fallback is hand-written handlers,
  held to the spec by the same contract tests.

**Conventions** (Zalando rule numbers in brackets):
- **Paths:** `/api`, with no version in the path [115]. Resources are
  plural nouns, and path segments are `kebab-case` [129]. JSON properties
  are `snake_case` [118].
- **`info`:**
  - `x-api-id`, a fixed UUID [215];
  - `x-audience: company-internal` [219];
  - `version`, the API's own semantic version, from `1.0.0`, with a minor
    bump for each addition [116].

  The ruleset gains rules for [215] and [219], with self-tests.
- **Lists** use cursor pagination [160]. Each takes `limit` (default 100,
  at most 1000) and an opaque `cursor`. It returns Zalando's page object
  [248], with `items`, `self` and `next`.
- **Links** are absolute [217]. They're built from the bootstrap key
  `server.public_url`, or, if that isn't set, from the request's host, over
  `http`.
- **Errors** are `application/problem+json` (RFC 9457) [176], for anything
  under `/api`, unknown paths and methods included.
- **Tracing:** each request takes an `X-Flow-ID`, or gets one, and returns
  it [233]. Its ID is the request ID in the logs. An incoming W3C
  `traceparent` continues the trace.
- **Times** are RFC 3339, in UTC [169]. Null and absent mean the same
  [123].

**Resources**, each a `GET`:
- `/api/status`: the service.
- `/api/server-groups` and `/api/server-groups/{group}`.
- `/api/server-groups/{group}/zones`, filtered by state, and
  `/api/server-groups/{group}/zones/{zone}`.
- `…/zones/{zone}/changes`: the RRsets that differ.
- `…/zones/{zone}/rrsets`: the zone's RRsets as NetBox defines them, in
  nbpdns's normalized form, with `as_of`, NetBox's last successful read.
- `/api/openapi.yaml` and `/api/docs` (ADR-0034).

Everything is last-known state. A group's drift is as of its last
successful read, and records are as of NetBox's. To serve records, `serve`
reads the RRsets of every active NetBox zone in the groups' views, not
only those it compares. `nbpdns drift` reads no more than before.

**The deviation from Zalando [104]**, "MUST secure endpoints": until M10,
the spec declares `security: [{}]`. That states plainly that there's no
authentication, rather than hiding it. M10 replaces it with its token
scheme. Until then, the API is for trusted networks, as `/status` and
`/metrics` are.

### Consequences

- Good: the API can't drift from its spec. The compiler checks the
  handlers against the generated interfaces, and the tests check every
  response.
- Good: IaC can read records now, and M18's resources can use the same
  stable IDs.
- Good: every list pages the same way, and every error has the same shape.
- Bad: the API is unauthenticated until M10, and names groups, zones and
  records.
- Bad: keeping NetBox's zones in memory costs memory, and reading every
  active zone costs requests for zones that are missing or ignored.
- Bad: oapi-codegen's 3.1 support is young, which limits the spec to plain
  3.1.

### Confirmation

- `make api-lint` passes on `api/openapi.yaml`, and its self-test covers
  [215] and [219].
- `make generate-check` passes, so the generated code and the embedded
  spec match the spec.
- The handler tests validate every response against the spec, and a
  coverage test fails if any operation or documented response code is
  never exercised.
- An integration test validates every resource against the spec, on the
  lab.

## Pros and cons of the options

### ogen

- Good: fast, with generated validation.
- Bad: its own router and runtime, beside the service's standard
  `ServeMux`.

### Hand-written handlers

- Good: no generator.
- Bad: only the tests link the code to the spec. A new field or parameter
  can be missed until a test catches it.

### kin-openapi's validator

- Good: oapi-codegen already uses kin-openapi.
- Bad: its 3.1 support is newer than libopenapi-validator's, which is
  built for 3.1 by the author of vacuum, the spec's linter.

### Records from both sides

- Good: a client could read what each primary serves.
- Bad: about twice the memory. A drifted zone's changes already show
  PowerDNS's side.

### No API until M10

- Good: nothing unauthenticated.
- Bad: M10's authentication would have nothing to protect, and the web UI
  and IaC wait longer. `/status` is already unauthenticated, on the same
  trusted listener.

## More information

- ADR-0012 (the API standard), ADR-0029 (the service), ADR-0034 (the
  viewer).
- Recorded in M06's design, 2026-10-08. Implemented by ITEM-0066 to
  ITEM-0072.
