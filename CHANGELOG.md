# Changelog

All notable changes to this project are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- An API, at `/api` on `server.listen`, described by its OpenAPI 3.1
  document, `api/openapi.yaml`, which the service serves at
  `/api/openapi.yaml`. `/api/status` gives the service's state, and
  `/api/server-groups` each group's configuration and last-known state,
  in pages, with `limit` and an opaque `cursor`, and absolute `self` and
  `next` links. Every error is an RFC 9457 problem. Each request takes, or is given, an
  `X-Flow-ID`, which is its request ID in the logs, continues the client's
  W3C `traceparent`, and is counted in `nbpdns_api_requests_total` and
  `nbpdns_api_request_duration_seconds`. `server.public_url` sets the host
  of the API's links, for a service behind a proxy. The API only reads,
  and has no authentication until M10, so keep it on a trusted network.

### Fixed

- A release's notes on GitLab are its version's section of the CHANGELOG.
  The 0.1.0 release was published with empty notes.

## [0.1.0] - 2026-10-07

### Added

- Releases. A version tag's pipeline publishes nbpdns as static Linux
  amd64 and arm64 binaries, in archives with `checksums.txt`, to a GitLab
  release, and as a multi-arch, non-root, distroless static image with an
  SPDX SBOM, to the GitLab registry. Every other pipeline builds and tests
  the same release without publishing it (`make release-check`). One commit
  builds the same archives anywhere.
- Documentation: how-to guides on installing nbpdns, and on running it in
  a container, with Docker or Kubernetes, a reference for the release
  artifacts, an explanation of how nbpdns is built and released, and a
  contributor's guide to making a release.
- `nbpdns serve` runs continuously. It refreshes the drift report every
  `drift.interval`, bounded by `drift.timeout`, and keeps each server
  group's last-known state when its primary, or NetBox, can't be read. At
  `server.listen` it serves `/livez`, `/readyz` (ready once the first
  refresh has finished), `/status`, and Prometheus metrics at `/metrics`.
  The status page shows the schedule, the outcome of each refresh,
  NetBox's and each group's state, the names of drifted zones, and the
  trace export, as text, or as JSON with `?json=1`. The metrics cover drift per group and
  per drifted zone, refreshes, and requests to NetBox and to each primary,
  and their reference is generated from the code. Each refresh is its own
  trace. It stops cleanly on SIGINT or SIGTERM.
- Documentation: a tutorial on running nbpdns as a service, how-to guides
  on monitoring drift with Prometheus, with example alert rules, and on
  exporting traces to an OpenTelemetry collector, an explanation of how
  nbpdns runs as a service, and references for the metrics and the service
  endpoints.
- Spans are exported to an OpenTelemetry collector over OTLP, by
  HTTP/protobuf or gRPC (`otlp.protocol`), from every command, when
  `otlp.endpoint` is set. Headers, such as the collector's token, are a
  secret (`otlp.headers`, with `_FILE`), and a CA file and a timeout are
  supported. A command sends its last spans as it ends. An `http://`
  endpoint works, with a warning.
- `nbpdns drift`: compare the zones NetBox assigns to each PowerDNS server
  group, through its views, with what the group's primary serves, and report
  every difference as a table or JSON, for every group or one (`--group`),
  and every zone or one (`--zone`). Only what each side serves is compared,
  and an SOA without its serial. Zones on a primary that NetBox doesn't
  assign to its group are listed as unmanaged, not drift, and zones whose
  policy is `ignore` aren't compared. The table gives each drifted zone's
  policy, with `enforce` marked as acting from M13. It exits 0 with no
  drift, 3 with drift, and 1 when NetBox or a primary can't be read. A
  group whose primary can't be read is marked failed, and the others are
  still reported. Server groups are read and compared concurrently, up to
  `drift.group_concurrency` at once.
- Documentation: a tutorial on finding drift between NetBox and PowerDNS, a
  how-to guide on setting a zone's drift policy, and an explanation of how
  nbpdns finds drift.
- `nbpdns netbox check`, `zones`, and `records`: read DNS data from the NetBox
  DNS plugin, through NetBox's REST API, with a read-only token. `check`
  reports NetBox's and the plugin's releases, and whether the token can view
  each kind of object. `records` shows a zone in nbpdns's normalized form:
  RRsets with absolute names, canonical values, and one TTL each, with
  inactive records listed. It reports problems in NetBox's data as warnings,
  and in its JSON output, without failing. A zone name in more than one view
  needs `--view`.
- `nbpdns powerdns check`, `zones`, and `records`: read each PowerDNS server
  group's primary, as a table or JSON, for every group or one (`--group`).
  `check` reports each primary's release and whether it accepts the key and
  lists its zones. `records` shows a zone in the same normalized form as
  `nbpdns netbox records`, with records PowerDNS doesn't serve listed as
  `disabled`.
- `nbpdns netbox zones --group` lists the NetBox zones a server group serves,
  through its views, and warns about a zone name that's in two of them.
- Documentation: a tutorial on reading PowerDNS zones, how-to guides on
  connecting nbpdns to PowerDNS and on putting the PowerDNS API behind a TLS
  proxy, and an explanation of how nbpdns reads PowerDNS.
- PowerDNS Authoritative 5.1 is supported, read through each server
  group's primary's HTTP API with its API key. Requests have a time limit and
  are retried when the failure may pass, redirects aren't followed, and TLS,
  a CA file, and a client certificate are supported. Plain `http://` works,
  with a warning. A server that isn't authoritative is an error, and an
  unsupported release a warning.
- PowerDNS server groups, declared in the config file under
  `powerdns.groups`: each a name, the NetBox views it serves, and its
  primary's API URL, API key or key file, server ID, CA file, and client
  certificate. Fields are checked strictly, every problem is reported at
  once, and `nbpdns config show` lists each field with the key redacted.
  `powerdns.timeout` and `powerdns.concurrency` set how requests to PowerDNS
  behave. Each group also sets the drift policy of its zones, `report` by
  default, with `zone_policies` for single zones: `enforce`, `report` or
  `ignore`.
- Record values of every type are normalized into one canonical form, parsed
  with the miekg/dns library, so that the same data reads the same from
  NetBox and from PowerDNS: names inside values lowercase and absolute, hex
  uppercase. A number too big for its field, such as an `SRV` port of 70000,
  is reported as a problem rather than accepted.
- NetBox 4.7, with the DNS plugin 1.7.x, is supported.
  Lists are paged, records are read with bounded concurrency, and requests
  have a time limit and are retried when the failure may pass. TLS 1.2 or
  later is required, with an optional CA file. Plain `http://` works, with a
  warning, and so do v1 tokens.
- Documentation: a tutorial on reading NetBox's DNS data, how-to guides on
  read-only access to NetBox and on configuring nbpdns, explanations of how
  nbpdns reads NetBox and of configuration precedence, and a supported
  versions reference.

- The `nbpdns` command, built as a static binary (`make build`), with
  `version`, `config show`, and shell completion. Output is a table or JSON
  (`--output`), and exit statuses tell success, failure, and usage errors
  apart.
- Configuration from flags, `NBPDNS_` environment variables, and a YAML config
  file, in that order of precedence. Secrets can be read from files (`_FILE`),
  and are redacted everywhere they could be printed or logged. A secret in
  the config file must be a string: one that YAML reads as a number is an
  error, rather than silently changed. Unknown keys and
  variables are errors, and every problem is reported at once.
- Configuration and command-line reference pages, generated from the code.
- A development lab with NetBox 4.7 and the NetBox DNS plugin, and a PowerDNS
  5.1 server, in containers (`make lab-up`, `make lab-down`). Integration
  tests run against it (`make test-integration`), in every pipeline.
- Logs on standard error, as JSON or text (`log.format`) from a chosen level
  (`log.level`). Every line carries the run's `trace_id`, `span_id`, and
  `request_id`, and outgoing requests carry the W3C `traceparent` header.
- Project foundation:
  - working rules for contributors (`CLAUDE.md`);
  - in-repo work tracking (`project/`);
  - architecture decision records ADR-0001 to ADR-0004;
  - the documentation skeleton and style guide.
- Decision records ADR-0005 to ADR-0009: project identity (`nbpdns`), the
  NetBox DNS plugin and PowerDNS-API-only integrations, server groups with
  catalog zones, drift policy per zone, and PostgreSQL/SQLite persistence with
  HA.
- Apache-2.0 license.
- Decision records ADR-0011 (Hugo for documentation) and ADR-0012 (API
  standard: OpenAPI 3.1 spec-first, Zalando guidelines with no URI versioning).
- Go module `github.com/zeddD1abl0/netbox-powerdns-ai` on Go 1.27.1. Development tools
  are pinned as release binaries by SHA-256 in `tools/tools.mk` (ADR-0014),
  except govulncheck and goimports, which are built from `tools/<name>/go.mod`.
- Documentation site built with Hugo and the vendored Hextra theme
  (`make docs`, `make docs-serve`), with internal links checked by htmltest.
- `make` as the single entry point (`make help` lists the targets). CI for
  GitLab and GitHub runs in stages (lint, test, build, security). Each job runs
  make targets in a digest-pinned Go image, and `make project-lint` keeps the
  jobs in step with `make ci`.
- Linting: golangci-lint (`make lint`), Vale with the vendored Google style
  (`make docs-lint`), and a self-tested Zalando ruleset for OpenAPI
  (`make api-lint`).
- Per-item commits on milestone branches (ADR-0010), with a hook that blocks
  commits on `main`.
