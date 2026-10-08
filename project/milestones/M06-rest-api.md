---
id: M06
title: REST API
status: in-progress # planned | in-progress | done
started: 2026-10-08
closed:
---

# M06: REST API

## Goal

A documented, contract-tested, read-only API at `/api`. It serves each
server group's drift, its zones and their changes, the zones' DNS records
as NetBox defines them, and the service's status. `api/openapi.yaml` is
the API's source of truth. The binary serves the spec, and a Scalar
reference at `/api/docs`.

## Non-goals

- **No authentication** until M10: the API is for trusted networks, as
  `/status` and `/metrics` are.
- **No writes:** no refresh trigger (M07 brings triggered refreshes), and
  no configuration through the API (M08 and later).
- **No history:** only each group's last-known state. Drift history is
  M08.
- **No problem or warning lists**, only their counts. The lists stay in
  `nbpdns drift`'s output.
- **No PowerDNS-side records resource:** a drifted zone's changes show
  what the primary serves.
- **No client SDK, provider or MCP server:** the provider is M18, and an
  MCP server is much later. Both build on the spec.
- **No caching validators** (`ETag` and `If-None-Match`) on reads. They can
  be added later without breaking anything.

## Phases

| Phase | Items |
|---|---|
| M6a Pipeline | ITEM-0066 The OpenAPI pipeline: pinned oapi-codegen, the spec's skeleton, the spec served and embedded, contract tests, and the ruleset's [215] and [219] rules |
| | ITEM-0067 The API's middleware: problem details, `X-Flow-ID` and tracing, request logs and metrics, and `server.public_url` |
| M6b Resources | ITEM-0068 Service status and server groups, with cursor pagination |
| | ITEM-0069 Zones and their changes |
| | ITEM-0070 DNS records from NetBox, with the full NetBox read in `serve` and a scale check |
| M6c Viewer and docs | ITEM-0071 Scalar at `/api/docs`, vendored and locked down |
| | ITEM-0072 The API docs: the generated reference, the how-to, the explanation, and the CHANGELOG |

## Acceptance criteria

- [x] ADR-0033 and ADR-0034 are accepted. Q-041 is answered, REQ-046 and
  REQ-047 exist, and ITEM-0065 is won't-fix.
- [x] oapi-codegen is pinned, and `make generate` and `generate-check`
  cover the generated server and the embedded spec. `api/openapi.yaml`
  passes `make api-lint`, whose self-test covers the new [215] and [219]
  rules.
- [x] Every resource in the table answers as specified, and every
  response, errors included, is validated against the spec in the handler
  tests. The coverage test passes.
- [x] Lists page with `limit` and `cursor`, with absolute `self` and
  `next` links, and errors are problem details everywhere under `/api`.
- [x] Each request returns its `X-Flow-ID`, continues an incoming
  `traceparent`, is logged with its request ID, and is counted in the new
  metrics.
- [x] `serve` keeps NetBox's zones, and `rrsets` serves them. `nbpdns
  drift` reads no more than before. The scale check's memory is recorded.
- [x] `/api/docs` serves the vendored Scalar with the CSP, and loads with
  no request to another host.
- [x] The integration test validates every resource against the spec, on
  the lab.
- [x] The docs above exist, the references are regenerated, and the
  CHANGELOG is updated.
- [x] `/code-review high` has run, and `/security-review` too, since M06
  opens an unauthenticated API.
- [ ] The manual verification is recorded. The pipelines pass, and the user
  has merged through an MR with a merge commit.

## Decided after approval

> [!IMPORTANT]
> Changed during implementation, with the reasons recorded in the items
> named. These override the approved design below.
>
> - **`/api/status` arrived with the pipeline**, in ITEM-0066, as its first
>   operation, so the pipeline was proven end to end. ITEM-0068 kept the
>   server groups and the paging.
> - **Spec details:**
>   - each zone has an `rrset_count`;
>   - the three, then four, page schemas share `PageLinks` by `allOf`;
>   - nullable enums are a `oneOf` of a named enum and `null`;
>   - the zone path parameter's component is `ZoneName`;
>   - every property has an example.
>
>   These are oapi-codegen's and vacuum's needs (ITEM-0066, ITEM-0069,
>   ITEM-0070).
> - **`make generate` runs oapi-codegen first**, since `gendocs` compiles
>   the module, which fails while the generated interface is ahead of the
>   handlers (ITEM-0069).
> - **Scalar's styles need no `'unsafe-inline'`:** Scalar reads a nonce
>   from a `<meta property="csp-nonce">`, so each response carries its own
>   nonce (ITEM-0071).
> - **Scalar's bundle is 4.4 MB, not about 3 MB.** It's vendored and
>   embedded gzipped, 1.28 MB, and served gzipped with ETags. Its license
>   comes from Scalar's repository at a pinned commit, since the npm
>   package has none, and the repository tags no package's releases
>   (ITEM-0071, ITEM-0073).
> - **The records' reads**, after the code review (ITEM-0073):
>   - `serve` keeps NetBox's inactive zones too, bare, so the records come
>     from the view the drift report compares;
>   - it reads the zones it doesn't compare in a call of their own, whose
>     failure keeps the last records and fails nothing else.
> - **Requests' metrics count `openapi` and `docs`** for the document and
>   the reference, besides each `operationId` and `unmatched` (ITEM-0067,
>   ITEM-0071).
> - **DNS rebinding against the unauthenticated listener** is ITEM-0074,
>   for M10.

## Verification log

Append-only and dated. Record what was run and what was seen.

- 2026-10-08: **The pipeline, proven first** (ITEM-0066). A draft spec
  with every construct M06 needed went through oapi-codegen v2.8.0 into a
  strict `net/http` server that compiled against runtime v1.7.0.
  libopenapi-validator v0.16.0 passed conforming responses, and caught a
  missing field, a value outside an enum, a number for a nullable string,
  and an undocumented content type. ADR-0033 needed no fallback.
- 2026-10-08: **Scale (REQ-043)** (ITEM-0070). A lab-only script rebuilt
  M03's data set: 1,000 zones of 100 A records in a NetBox view, `scale`,
  and 995 of them on lab-a, with 10 changed values, 5 extra TXT RRsets and
  5 zones missing. `serve` ran five refreshes at a 70 s interval:
  - every refresh was complete, finding 980 in sync, 15 in drift and 5
    missing, exactly the planted drift;
  - they averaged 53.0 s, against M04's 54.7 s, with about 1,000 NetBox
    requests each, one per zone, and no retries;
  - memory sat at 76 to 79 MB between refreshes, with NetBox's 100,000
    records kept, against M04's 38 MB, and peaked at 131 to 132 MB at each
    refresh's end, against M04's 95 MB, with no growth over the five;
  - the API answered 1,000 zones in one page in 8.4 ms, the drifted ones
    in 7.6 ms, a zone's 102 RRsets in 0.5 ms, and `/api/status` in
    0.3 ms;
  - SIGTERM: exit 0 in 27 ms.

  Afterwards, the data set's zones were removed from lab-a, and NetBox's
  are being deleted, which is as slow as their creation; `make lab-down`
  would reset the lab anyway.
- 2026-10-08: **The how-to, run** (ITEM-0072). Every command in "Read drift
  and DNS records through the API" ran against that `serve`, and gave
  the planted drift:
  - the drifted zones across groups;
  - z0000's changed A record, and z0010's extra TXT;
  - the missing zone's 100 records as zone-file lines;
  - 102 RRsets paged in four;
  - a flow ID echoed and logged.
- 2026-10-08: **The reference in a browser** (ITEM-0071). Headless Firefox,
  with an isolated profile, loaded `/api/docs` from `serve` on the lab. It
  rendered styled, with the spec's operations, models and server URL,
  under the strict CSP with no `'unsafe-inline'`, in the browser's own
  fonts.
- 2026-10-08: **Flow IDs and traces.** A request with `X-Flow-ID:
  m06-verify-1` and a `traceparent` went to `serve`, with spans exported
  to Jaeger (`jaegertracing/jaeger:latest`, a temporary container):
  - the response returned the flow ID;
  - its log line had it as `request_id`, with the incoming trace's ID;
  - Jaeger had the span `GET /api/server-groups`, a child of the incoming
    parent span, with `http.route` and status 200.
- 2026-10-08: **`/code-review high`** on `origin/main...m06-rest-api` at
  `f4871f1` (a first attempt stopped at the session limit) found ten
  things (ITEM-0073):
  1. an empty NetBox read never replaced the records;
  2. HEAD requests lost their route, and miscounted the reference;
  3. links lost a zone name's escapes, so an RFC 2317 zone couldn't be
     paged;
  4. records could come from another view than the report's;
  5. a failed read of a zone that isn't compared failed the refresh;
  6. zone lists mapped every zone before paging;
  7. duplicated cursor and zone-name code;
  8. Scalar's license fetched from a moving branch;
  9. links from the request's Host when `server.public_url` isn't set;
  10. `gzip;q=0`, and ETag lists and weak tags, ignored.

  Nine were fixed. The ninth stays as ADR-0033 designed it, documented:
  responses are `no-store`, so a forged Host misleads only its own
  request.
- 2026-10-08: **`/security-review`** on the branch, with the fixes: nothing
  at the report's bar. It checked:
  - **Secrets:** none reaches a response, a log line, a span or the
    reference. The status fields are those `/status?json=1` already
    served.
  - **The reference page:** html/template, a `crypto/rand` nonce, the CSP
    with nothing `unsafe`, `nosniff`, and correct content types.
  - **Problems:** always `application/problem+json`, HTML-escaped by
    `encoding/json`.
  - **Flow IDs:** anchored, with no CR or LF.
  - **Links:** a forged Host can't poison a cache, since every response is
    `no-store`.
  - **Cursors:** they decode into two strings, nothing more.
  - **Docs assets:** a map lookup of three files, so no path traversal.
  - **Scalar:** the bundle is pinned and verified, and matched the npm
    tarball byte for byte.

  Below its bar: DNS rebinding against the unauthenticated listener, as
  `/status` has been open to since M04, is ITEM-0074, for M10.
- 2026-10-08: **Close checks**, on the tree committed as `baaed89`:
  - `make check` passes: vet, golangci-lint with 0 issues, the tests with
    `-race`, govulncheck, gitleaks, Vale, the API ruleset's self-test and
    the spec at 100/100, project lint, and `generate-check`, which now
    covers the API's code, its embedded spec and its reference.
  - `make test-integration` passes against the local lab, with `TestServe`
    and `TestDrift` exercising the API.
  - `make release-check` passes.
  - `make docs-links` passes, on 73 pages.
  - The stripped binary is 21.1 MB, from 19.8 MB at M05's end: +1.31 MB
    for Scalar's gzipped bundle, and the API.
- 2026-10-08: The scale data set is gone from the lab: NetBox's `scale`
  view, its 1,000 zones and their records, and the name server. lab-a is
  back to its one zone.

## Approved design

The plan approved on 2026-10-08, copied verbatim. Its headings are demoted two
levels to nest under this section; the text is unchanged. It's a snapshot.
Where it disagrees with an ADR or `CLAUDE.md`, they win.

### M06: REST API

#### Context

M04 made nbpdns a service that keeps a drift report current, and M05
released it as v0.1.0. Its state can only be read as `/status` and the
metrics, which were built for people and Prometheus, not as a contract.
M06 adds the API that later milestones build on: the web UI (M12), the
Terraform/OpenTofu provider (M18) and the Ansible collection (M19). It's
spec-first OpenAPI 3.1, under the Zalando guidelines (ADR-0012). It's
read-only, and unauthenticated until M10, so only for trusted networks.

Decided with the user in the M06 design session, 2026-10-08:
- **Q-041 (what IaC manages):** nbpdns's own configuration, plus reading
  DNS records. Terraform should be able to read a zone's records from
  nbpdns, and push them to other providers. Records are still written only
  in NetBox.
- **Resources:**
  - server groups;
  - their zones, with each zone's changes;
  - the zones' DNS records, as NetBox defines them;
  - the service's status.

  There's no refresh trigger: that would change state without
  authentication.
- **Records** are in M06, from NetBox only. PowerDNS's side shows in each
  drifted zone's changes.
- **The viewer at `/api/docs`** is Scalar, vendored and locked down:
  - default fonts off;
  - Agent Scalar off;
  - a strict same-origin Content-Security-Policy, so the page can't reach
    any other host.
- **ITEM-0065 (the slow lab start on GitLab's runner) is won't-fix.**
  It's the runner's hardware: memory paging when the node is busy. Retries
  pass, and the runner can't be enlarged now.

#### Goal

A documented, contract-tested, read-only API at `/api`. It serves each
server group's drift, its zones and their changes, the zones' DNS records
as NetBox defines them, and the service's status. `api/openapi.yaml` is
the API's source of truth. The binary serves the spec, and a Scalar
reference at `/api/docs`.

#### Non-goals

- **No authentication** until M10: the API is for trusted networks, as
  `/status` and `/metrics` are.
- **No writes:** no refresh trigger (M07 brings triggered refreshes), and
  no configuration through the API (M08 and later).
- **No history:** only each group's last-known state. Drift history is
  M08.
- **No problem or warning lists**, only their counts. The lists stay in
  `nbpdns drift`'s output.
- **No PowerDNS-side records resource:** a drifted zone's changes show
  what the primary serves.
- **No client SDK, provider or MCP server:** the provider is M18, and an
  MCP server is much later. Both build on the spec.
- **No caching validators** (`ETag` and `If-None-Match`) on reads. They can
  be added later without breaking anything.

#### Decisions

##### ADR-0033: a read-only API, spec-first, generated with oapi-codegen

- **The contract:**
  - `api/openapi.yaml`, OpenAPI 3.1, is the source of truth.
  - **oapi-codegen** v2.8+ (Apache-2.0), pinned in `tools/oapi-codegen/go.mod`,
    generates `net/http` strict-server interfaces and types into
    `internal/api/gen`. Its runtime, `github.com/oapi-codegen/runtime`
    (Apache-2.0), binds the parameters.
  - **Contract tests** check every response, errors included, against the
    spec, with pb33f's **libopenapi-validator** (MIT; OpenAPI 3.0 to 3.2).
  - `make generate` regenerates the code and an embedded copy of the spec,
    and `generate-check` fails if either is stale.
- **Why oapi-codegen** (ADR-0012's candidate):
  - it uses the standard library's `ServeMux`, which the service already
    uses;
  - it's widely used;
  - its 3.1 support arrived in 2.8, after kin-openapi gained it.

  ogen is faster, but brings its own router and runtime.
  Hand-written handlers would lose the compile-time link between the spec
  and the code.
- **The risk:** oapi-codegen calls its 3.1 support "initial". The spec
  keeps to plain 3.1: `type: [T, "null"]` for nullable fields, and no
  webhooks. ITEM-0066 proves the whole pipeline first. If oapi-codegen
  can't generate from the spec, the fallback is hand-written handlers,
  held to the spec by the same contract tests. That would be recorded
  under "Decided after approval".
- **Conventions** (Zalando):
  - **Paths:** `/api`, with no version in the path. Resources are plural,
    and path segments are `kebab-case`. JSON properties are `snake_case`.
  - **`info`:**
    - `x-api-id`, a fixed UUID [215];
    - `x-audience: company-internal` [219];
    - `version`, the API's own semantic version, from `1.0.0`, with a
      minor bump for each addition [116].
  - **Lists** use cursor pagination [160]. Each takes `limit` (default 100,
    at most 1000) and an opaque `cursor`, and returns Zalando's page
    object [248]: `items`, `self` and `next`.
  - **Links** are absolute [217]. They're built from the new bootstrap key
    `server.public_url`, or, if that isn't set, from the request's host,
    over `http`.
  - **Errors** are `application/problem+json` (RFC 9457) [176]. That covers
    400 for a bad parameter or cursor, 404 for an unknown group, zone or
    path, and 405, for anything under `/api`.
  - **Tracing:**
    - each request takes `X-Flow-ID`, or gets one, and returns it [233];
    - its ID is the request ID in the logs;
    - an incoming W3C `traceparent` starts the request's span.
  - **Times** are RFC 3339, in UTC [169]. Null and absent mean the same
    [123].
- **The deviation from Zalando [104]:** "MUST secure endpoints". Until M10,
  the spec declares `security: [{}]`: no authentication, stated in the
  spec rather than hidden. M10 replaces it with its token scheme. The
  ruleset is unchanged, since `[{}]` passes its check; this ADR records
  why.
- **The ruleset** gains rules for [215] `x-api-id` and [219]
  `x-audience`, with self-tests, since the first real spec arrives now.

##### ADR-0034: a vendored, locked-down Scalar viewer at /api/docs

- **The asset:** Scalar's API reference (MIT), the standalone bundle from
  `@scalar/api-reference`. It's vendored into `internal/api/docs/` with its
  licence. `make vendor-scalar` fetches a pinned version, and checks it by
  SHA-256. The binary embeds it: there's no CDN (ADR-0022).
- **Locked down:**
  - a same-origin init script sets `withDefaultFonts: false`, turns Agent
    Scalar off, and points at `/api/openapi.yaml`;
  - the page and its assets are served with a strict Content-Security-Policy
    (`default-src 'none'`, with `script-src`, `style-src`, `font-src`,
    `img-src` and `connect-src` limited to `'self'`, and `'unsafe-inline'`
    for styles only if Scalar needs it), and `frame-ancestors 'none'`;
  - a test checks the header.

  So the browser refuses any request to another host, even from a default
  that a later Scalar release adds.
- **Why Scalar:** its navigation, multi-language code samples and request
  client. What helps AI clients is the spec's quality, served at
  `/api/openapi.yaml`, not the viewer. Scalar's own AI features are
  hosted, and stay off.

##### Requirements and questions

- **Q-041** moves to Answered: IaC manages nbpdns's own configuration, and
  reads DNS records through the API. Records are written only in NetBox.
  It produces REQ-046 and REQ-047.
- **REQ-046:** nbpdns serves a read-only API, documented by
  `api/openapi.yaml`, of each server group's drift, its zones and their
  changes, and the service's status.
- **REQ-047:** the API serves each zone's DNS records as NetBox defines
  them, in nbpdns's normalized form, for IaC to read.
- **ITEM-0065** is closed as won't-fix, with the user's reason.

#### Design

##### Resources

Every resource is a `GET` under `/api`.

| Path | Returns |
|---|---|
| `/api/status` | The service: build, uptime, readiness, the refresh schedule and the last refresh's outcome, NetBox's state, and tracing. It's `/status?json=1` without the groups, which have their own resource. |
| `/api/server-groups` | A page of groups, in the configuration's order. Each has its name, primary URL, views and default drift policy, its status (`ok`, `failed` or `unknown`), error, `last_success`, zone counts by state, and problem and warning counts. |
| `/api/server-groups/{group}` | One group. |
| `/api/server-groups/{group}/zones` | A page of the group's zones, in canonical name order. Each has its zone, view, policy, state, SOA serials, and change and RRset counts. `?state=drift,missing` filters by state. Unmanaged zones are included, with the state `unmanaged`. |
| `/api/server-groups/{group}/zones/{zone}` | One zone. The name may be given with or without its final dot. |
| `/api/server-groups/{group}/zones/{zone}/changes` | A page of the zone's changed RRsets: name, type, kind, and each side's TTL and values. |
| `/api/server-groups/{group}/zones/{zone}/rrsets` | A page of the zone's RRsets as NetBox defines them, normalized: name, type, TTL, and active records, each with its value, and `managed` for NetBox's own SOA and NS records. It's 404 for a zone that isn't active in NetBox. The page has `as_of`, NetBox's last successful read. |
| `/api/openapi.yaml` | The spec. |
| `/api/docs` | The Scalar reference. |

- A group's drift is as of its `last_success`. A zone's RRsets are as of
  NetBox's last successful read.
- A cursor carries the last item's key and the request's filter. A cursor
  used with another filter is a 400.
- Pages are read at the time of the request, so a refresh between pages
  can add or remove items.

##### Code

- **`internal/api`:**
  - the generated server, in `internal/api/gen`;
  - handlers that map a snapshot of the service's state onto the
    generated types;
  - pagination;
  - middleware:
    - problem details for anything under `/api`;
    - `X-Flow-ID` and the request ID;
    - a span for each request, from `traceparent`;
    - a log line for each request;
    - request metrics.

  It reads the state through an interface, so tests can feed it
  fixtures.
- **`internal/service`** gains `Snapshot()`: a copy of the state, taken
  under its lock. It keeps each group's last report, as now, and NetBox's
  last-read zones, by view and name. The handler mounts `/api/` on the
  existing mux, at `server.listen`.
- **`internal/drift`:** `Options.ReadNetBox` makes `Run` read the RRsets of
  every active NetBox zone in the groups' views, not only those it
  compares, and return them in the report. `serve` sets it. `nbpdns drift`
  doesn't, so it reads no more than now. The extra reads are for zones
  that are missing or ignored, so they're few when the groups are in sync.
- **`internal/metrics`** declares `nbpdns_api_requests_total{operation,code}`
  and `nbpdns_api_request_duration_seconds{operation}`. Their reference is
  regenerated.
- **`internal/config`** declares `server.public_url`. Its reference is
  regenerated.
- **`internal/cmd/gendocs`** generates `docs/reference/api.md` from the
  spec: every operation, parameter, response and schema field, with its
  description.

##### Tests

- **Table-driven handler tests** for each operation, over fixture
  snapshots, with `-race`. Every response, errors included, is validated
  against the spec. A coverage test fails if any operation, or any
  documented response code, is never exercised.
- **Pagination:** limits, cursors across pages, a cursor reused with
  another filter, the final page with no `next`, and absolute links with
  and without `server.public_url`.
- **Integration** against the lab: `serve`, then every resource, each
  validated against the spec. A zone drifted on lab-a shows in `zones`,
  `changes` and `/api/status`. A zone's `rrsets` match NetBox's records,
  as `nbpdns netbox records` shows them.
- **The viewer:** `/api/docs` and its assets are served with the CSP, and
  the embedded page references only same-origin URLs.

##### Docs

| Section | Pages |
|---|---|
| Reference | "API" (`api.md`, generated from the spec). "Service endpoints" gains `/api`, `/api/openapi.yaml` and `/api/docs`. The configuration and metrics references are regenerated. |
| How-to | "Read drift and DNS records through the API": list the drifted zones across groups with curl and jq, page through a large zone, and fetch a zone's records for another tool. |
| Explanation | "How nbpdns's API is designed": spec-first, compatibility with no versions in paths, pagination and cursors, problem details, last-known state and freshness, unauthenticated until M10, and the locked-down viewer. |

The CHANGELOG gets lines under Unreleased. The Vale vocabulary gains the
new names.

#### Items and phases

| Phase | Items |
|---|---|
| M6a Pipeline | ITEM-0066 The OpenAPI pipeline: pinned oapi-codegen, the spec's skeleton, the spec served and embedded, contract tests, and the ruleset's [215] and [219] rules |
| | ITEM-0067 The API's middleware: problem details, `X-Flow-ID` and tracing, request logs and metrics, and `server.public_url` |
| M6b Resources | ITEM-0068 Service status and server groups, with cursor pagination |
| | ITEM-0069 Zones and their changes |
| | ITEM-0070 DNS records from NetBox, with the full NetBox read in `serve` and a scale check |
| M6c Viewer and docs | ITEM-0071 Scalar at `/api/docs`, vendored and locked down |
| | ITEM-0072 The API docs: the generated reference, the how-to, the explanation, and the CHANGELOG |

Once the plan is approved, one commit records the design:
- this plan, verbatim, under **Approved design** in M06's file;
- ADR-0033 and ADR-0034;
- REQ-046 and REQ-047, and Q-041 answered;
- the brief's dated answers;
- ITEM-0065 won't-fix;
- M06 marked in progress;
- the items.

Each item is then committed on `m06-rest-api` as it's done.

#### Acceptance criteria

- [ ] ADR-0033 and ADR-0034 are accepted. Q-041 is answered, REQ-046 and
  REQ-047 exist, and ITEM-0065 is won't-fix.
- [ ] oapi-codegen is pinned, and `make generate` and `generate-check`
  cover the generated server and the embedded spec. `api/openapi.yaml`
  passes `make api-lint`, whose self-test covers the new [215] and [219]
  rules.
- [ ] Every resource in the table answers as specified, and every
  response, errors included, is validated against the spec in the handler
  tests. The coverage test passes.
- [ ] Lists page with `limit` and `cursor`, with absolute `self` and
  `next` links, and errors are problem details everywhere under `/api`.
- [ ] Each request returns its `X-Flow-ID`, continues an incoming
  `traceparent`, is logged with its request ID, and is counted in the new
  metrics.
- [ ] `serve` keeps NetBox's zones, and `rrsets` serves them. `nbpdns
  drift` reads no more than before. The scale check's memory is recorded.
- [ ] `/api/docs` serves the vendored Scalar with the CSP, and loads with
  no request to another host.
- [ ] The integration test validates every resource against the spec, on
  the lab.
- [ ] The docs above exist, the references are regenerated, and the
  CHANGELOG is updated.
- [ ] `/code-review high` has run, and `/security-review` too, since M06
  opens an unauthenticated API.
- [ ] The manual verification is recorded. The pipelines pass, and the user
  has merged through an MR with a merge commit.

#### Verification

- `make check` on every commit, and `make test-integration` and
  `make release-check` locally.
- **Manual:**
  1. Run `serve` against the lab, and read every resource with curl:
     - the drifted zone across `zones` and `changes`;
     - its `rrsets` against `nbpdns netbox records`;
     - pages with `limit=1`;
     - problem details for a bad cursor, an unknown group, and a `POST`.
  2. Validate the live responses against the spec, through the
     integration test.
  3. Load `/api/docs` in headless Firefox. Check that it renders the
     reference, as a screenshot, and that the CSP header is on the page
     and its assets.
  4. Scale (REQ-043): run M03's data set, 1,000 zones, under `serve`.
     Record each refresh's time and the resident memory with NetBox's
     zones kept, against M04's 95 MB peak. Then page through a large
     zone's `rrsets`.
  5. `X-Flow-ID` and `traceparent`: send both, and find the request's
     span in Jaeger, and its request ID in the logs.

#### Your side

- Keep `server.listen` on a trusted network: the API is unauthenticated
  until M10, and names your groups, zones and records.
- After the merge, there's no release unless you want one. A `v0.2.0` tag
  would publish the API, following "Make a release".
