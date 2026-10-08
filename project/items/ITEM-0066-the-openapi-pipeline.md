---
id: ITEM-0066
title: The OpenAPI pipeline
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M06
requirements: [REQ-019, REQ-037, REQ-046]
depends_on: []
created: 2026-10-08
closed: 2026-10-08
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

- [x] oapi-codegen is pinned in `tools/oapi-codegen/go.mod`; `make generate` writes `internal/api/gen` and the embedded spec, and `generate-check` fails if either is stale.
- [x] `api/openapi.yaml` has `info` with `x-api-id`, `x-audience` and `version` 1.0.0, `security: [{}]`, the Problem schema and the page object, and passes `make api-lint`.
- [x] The ruleset has rules for [215] and [219], and `api/testdata` self-tests them.
- [x] `/api/openapi.yaml` serves the spec, and a contract-test helper validates any response against it.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M06's approved design.
- 2026-10-08: Done.
  - **A spike first,** in the scratchpad, on a draft that used every
    construct M06's spec needs. oapi-codegen v2.8.0 generated a strict
    `net/http` server from it, which built against its runtime, v1.7.0.
    Nullable `type: [T, "null"]` became pointers, and a `oneOf` with `null`
    a pointer to the type. The comma-separated array query parameter, the
    header parameters, and shared responses with headers all generated.
    libopenapi-validator v0.16.0 passed conforming responses, and caught a
    missing field, a value outside an enum, a number where only a string or
    null is allowed, and an undocumented content type. So there's no
    fallback: ADR-0033 stands as written.
  - **Two quirks, handled in the spec and the config:**
    - a nullable enum written as `enum: [..., null]` generates a constant
      named `LessThannil`, so nullable enums are a `oneOf` of a named enum
      schema and `null`, such as `OTLPProtocol`;
    - enum constants aren't prefixed by default, so the zone state
      `missing` would collide with the change kind `missing`.
      `always-prefix-enum-values` names them `RefreshOutcomeComplete` and
      so on.
  - **The pipeline:**
    - oapi-codegen is pinned in `tools/oapi-codegen/go.mod`, and
      `make tools-update` moves it with the other source-built tools.
    - `api/oapi-codegen.yaml` configures it. It names no output, because
      `-o` is ignored when the config names one.
    - `make generate` writes `internal/api/gen/api.gen.go` and
      `internal/api/openapi.yaml`, the spec's embedded copy, with a header
      naming its source. `make generate-check` regenerates both into a
      temporary directory, and fails if either differs.
  - **The spec** has its `info` (`x-api-id`, `x-audience:
    company-internal`, version 1.0.0, the licence), `servers: /api`,
    `security: [{}]` (ADR-0033's deviation from [104]), the shared
    `X-Flow-ID` parameter, header and problem response, and `Problem`. It
    has `GET /status`.
    - Every property has `examples`, which vacuum's `oas3-missing-example`
      asks for one property at a time. It's also what the generated
      reference and AI clients use.
    - `make api-lint` scores 100/100, with no warnings.
  - **`/api/status` arrived here,** not in ITEM-0068, as the pipeline's
    first operation, so the pipeline is proven end to end. It maps
    `Service.Status()` onto the generated types, without the groups. A
    missing error, endpoint or protocol is `null`, not `""`.
    `/api/openapi.yaml` serves the embedded spec as `application/yaml`.
    Every response under `/api` is `Cache-Control: no-store`.
  - **The ruleset** gains `zalando-215-api-id` (present, and a UUID) and
    `zalando-219-audience` (present, and one of Zalando's five audiences).
    Each checks presence with `truthy` and the value with `pattern`, since
    vacuum's `enumeration` doesn't read extension fields through `field`.
    `api/testdata/bad.yaml` breaks both, and a spec without `x-audience`
    fails too.
  - **`internal/api/contract`** builds a checker from the spec with
    libopenapi-validator. It validates each response, and records the
    operation, matched by template with literal segments preferred, and
    its status code. `Missing()` lists the listed codes no test checked.
    - Its own tests show it rejecting each kind of bad response.
    - `internal/api`'s `TestMain` fails when a full run leaves anything
      missing.
    - The serve tests check `/api/status` through it: unit, with NetBox
      unreachable (`up: false`, no secret), and integration, on the lab.
  - **`serve`** mounts the API at `/api/` beside the service's handler.
  - **Dependencies** (licences checked from each module's licence file):
    - `github.com/oapi-codegen/runtime` v1.7.0 (Apache-2.0), with
      `apapsch/go-jsonmerge` (MIT), the only two that reach the binary;
    - for the tests only, `pb33f/libopenapi-validator` v0.16.0 and
      `pb33f/libopenapi` v0.41.3 (MIT), with their Apache-2.0, MIT and BSD
      dependencies.
  - **Docs:** "Service endpoints" lists `/api/status` and
    `/api/openapi.yaml`, and the CHANGELOG has the API under Unreleased.
    ITEM-0072 adds the generated reference.
