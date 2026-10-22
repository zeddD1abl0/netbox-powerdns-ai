# Changelog

All notable changes to this project are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- The development lab publishes its PostgreSQL 18 server on port 5418, and
  `make lab-up` creates a database in it for nbpdns, `nbpdns`, owned by the
  role `nbpdns`. The store's integration tests run on it as well as on
  SQLite, each test in a database of its own.

### Changed

- `make release-check` and `make release` build one architecture at a
  time, which lowers their peak memory by a third, and GitLab's
  memory-heavy jobs request memory from the Kubernetes runner, so that
  each runs only on a node with room for it.

### Security

- nbpdns is built with Go 1.27.3, which fixes GO-2026-6809: when nbpdns
  checks the certificate of a server that it connects to over TLS, such as
  NetBox or PowerDNS, Go now refuses a chain through any of the known
  intermediate certificates that are no longer trustworthy, even where the
  system's trust store hasn't been updated to distrust them.

## [0.8.0] - 2026-10-17

### Added

- `nbpdns serve` keeps its state in a database, embedded SQLite, at
  `database.url`, by default `sqlite:///var/lib/nbpdns/nbpdns.db`. It
  creates the database, readable only by its owner, and applies its
  migrations, when it starts. Only one `serve` may use a database at a
  time: a second one refuses to start, and names the first one's host and
  PID.
- `nbpdns db status` shows the database and its migrations, and `nbpdns db
  migrate` creates it, or applies the migrations it lacks.
- An audit trail, in the database: every change of nbpdns's state is an
  event, with its actor, action, target, values before and after, reason,
  and trace and request IDs, in the same transaction as the change. Each
  event holds the SHA-384 of its contents and of the event before it.
  `nbpdns audit verify` checks the chain, and `nbpdns audit list` reads
  the events. `nbpdns serve` records its start, with its configuration's
  fingerprint, and its stop, and deletes events older than
  `audit.retention_days`, by default seven years, daily.
  "Audit events" lists every event, and `nbpdns_audit_events_total` counts
  them.
- A drift history, in the database: each refresh, with each group's
  result, and each zone's row whenever its state or its RRset changes
  differ from its last, kept for `drift.history_retention_days`, by default
  400. A zone's change of state, and a new zone that drifted, are also
  `zone.drift_changed` audit events.
- `nbpdns serve` restores each group's last report from the database when
  it starts, and is ready at once, serving it until its first refresh
  replaces it. `/status` gains `database`, which says whether it restored,
  and counts the refreshes that it couldn't write to the history, as
  `nbpdns_db_write_failures_total` does.
- The API, now version 1.2.0, serves the drift history:
  `/api/server-groups/{group}/zones/{zone}/history`, a zone's rows, and
  `/api/server-groups/{group}/refreshes`, the refreshes, each with the
  group's result. Both list the newest first, in pages with cursors.
  `/api/status` gains `database`. `nbpdns drift history` shows a group's
  refreshes, or, with `--zone`, a zone's history, in a table, or in JSON
  with `--output json`. "Read a zone's drift history" shows how.
- Runtime settings: each key is a runtime or a bootstrap key, as the
  configuration reference shows. `nbpdns settings list|get|set|unset`
  shows and changes the runtime ones, which the database stores, between
  the defaults and the config file, and a running `nbpdns serve` applies
  within seconds, without a restart. A runtime key that the config file,
  the environment or a flag sets is pinned. Each change is a `setting.set`
  or `setting.unset` audit event, and `nbpdns_settings_reloads_total`
  counts serve's reloads. "Change nbpdns's settings while it runs" shows
  how.
- Secret runtime settings, such as `netbox.token`, are stored sealed with
  `database.master_key` (and its `_FILE` form), by envelope encryption with
  AES-256-GCM, each bound to its key. `nbpdns settings set` takes a secret
  only from `--from-file`, or standard input, and `nbpdns serve` refuses to
  start with a secret that it can't open. `nbpdns secrets
  rotate-master-key` re-wraps them with a new master key, in one audited
  transaction, after which a secret sealed with the old key is refused.
  "Protect stored secrets with a master key" shows how.
- Server groups in the database, each with its owner, `managed_by`:
  `nbpdns serve` mirrors the config file's groups into it at each start,
  and `nbpdns groups list|add|set|remove|rename` manages more, which a
  running `serve` compares, changes, or stops comparing within seconds. A
  group's API key comes from `--api-key-file` or `NBPDNS_GROUP_API_KEY`,
  and is stored sealed with the master key, which `rotate-master-key`
  covers. `add`, and a `set` that changes how nbpdns reaches the primary,
  check that it answers, unless `--no-check` is given. A rename keeps the
  group's drift history, and a removed group's history is kept until
  `drift.history_retention_days` prunes it. Each change is a `group.*`
  audit event. The API's server groups gain `managed_by`. "Manage server
  groups from the command line" shows how.
- `make tidy` tidies the `go.mod` and `go.sum` of every Go module, the
  tools' too, and `make tidy-check`, which `make check` and CI run, fails,
  naming each module that isn't tidy.
- `gopls` is pinned in `tools/gopls/go.mod`, and `make gopls` installs it
  where `go install` puts binaries, for Claude Code's `gopls-lsp` plugin,
  failing if it isn't the first `gopls` on `PATH`. `make tools-update`
  moves it to its latest release, and installs it again.

### Changed

- `/readyz` is ready once `nbpdns serve` has restored its last reports, or,
  with none to restore, once its first refresh has finished.
- `nbpdns config show` takes the runtime settings that the database stores
  into account, if the database exists, and names it as their source.
- `nbpdns serve` starts with no server groups, since `nbpdns groups add`
  can add them while it runs, where it used to refuse to start. The API
  no longer calls a group's name its stable ID: a rename changes it.
- `nbpdns serve` needs a directory that it can write its database to. In
  the image, mount a volume at `/var/lib/nbpdns`, writable by user 65532,
  and in Kubernetes, update the deployment with the `Recreate` strategy, as
  "Run nbpdns in a container" shows.
- `make secrets`, and so `make check`, also scan what isn't committed yet:
  changed and new files, whether they're staged or not. A secret then
  fails the scan before its commit, while it can still be taken out, not
  only after, when it can only be ignored.

### Fixed

- While a refresh runs, `/status` and `/api/status` show the last refresh
  with its own start. They showed the start of the one running with the
  end of the last one, and so a negative duration, which the contract of
  `/api/status` refuses; since the restore, the first refresh after every
  restart did.
- `nbpdns_netbox_up`, and `netbox.up` on `/status` and `/api/status`, no
  longer say that NetBox is up after a refresh that didn't read it: a zone
  refresh of zones that no server group serves, or, now that `nbpdns
  serve` starts without groups, a refresh with none to compare.

## [0.7.0] - 2026-10-09

The first release since 0.1.0, with the REST API of M06 and the
refreshes from NetBox's webhooks of M07.

### Added

- An API, at `/api` on `server.listen`, described by its OpenAPI 3.1
  document, `api/openapi.yaml`, which the service serves at
  `/api/openapi.yaml`. `/api/status` gives the service's state, and
  `/api/server-groups` each group's configuration and last-known state.
  A group's `zones` can be filtered by state, such as
  `?state=drift,missing`, and include the primary's unmanaged zones. A
  zone, by its name with or without the final dot, has its `changes`:
  each RRset that differs, with each side's TTL and values; and its
  `rrsets`: its records as NetBox defines them, in nbpdns's normalized
  form, with NetBox's own SOA and NS records marked `managed`, for IaC to
  read and publish elsewhere. `nbpdns serve` now reads the records of
  every active zone in the groups' views to serve them, which
  `nbpdns drift` doesn't. Lists come in
  pages, with `limit` and an opaque `cursor`, and absolute `self` and
  `next` links. Every error is an RFC 9457 problem. `/api/docs` is a
  browsable reference, Scalar's, built into nbpdns, whose
  Content-Security-Policy lets it reach no other host.
- Documentation: the API's reference, generated from its OpenAPI document,
  a how-to guide on reading drift and DNS records through the API with curl
  and jq, and an explanation of how the API is designed. Each request takes, or is given, an
  `X-Flow-ID`, which is its request ID in the logs, continues the client's
  W3C `traceparent`, and is counted in `nbpdns_api_requests_total` and
  `nbpdns_api_request_duration_seconds`. `server.public_url` sets the host
  of the API's links, for a service behind a proxy. The API only reads,
  and has no authentication until M10, so keep it on a trusted network.

- Refreshes from NetBox's webhooks. With `netbox.webhook_secret` set,
  `nbpdns serve` takes NetBox's event-rule webhooks at
  `/api/netbox-events`, signed with that secret, and compares only the
  zones they name, once they stop coming for `drift.webhook_delay`, 3
  seconds by default, or 30 seconds after the first. A change to a view
  that a group serves, a zone or a record moved, or more than 100 zones
  make a full refresh instead. The scheduled refresh still runs, for any webhook lost. Each
  zone refresh is a trace linked to the webhooks' spans, with NetBox's
  request IDs and users, which its log lines carry too. New metrics:
  `nbpdns_netbox_webhooks_total`, `nbpdns_drift_zone_refreshes_total`,
  `nbpdns_drift_zone_refresh_duration_seconds` and
  `nbpdns_drift_pending_zones`. `/status` and `/api/status` gain a
  `webhooks` section: whether they're on, the last event, what waits, and
  the last refresh they asked for. The API's version is 1.1.0. The how-to
  "Refresh drift as NetBox changes" sets them up in NetBox, by its web
  interface or its REST API, with the event rule in Terraform if you like.

- The development lab's profile `webhooks`, which adds NetBox's worker, so
  that the lab's NetBox sends webhooks: `make lab-up LAB_PROFILES=webhooks`.
  `make test-webhooks` uses it to have NetBox send webhooks to
  `nbpdns serve`, on a local Docker host. The integration tests replay
  NetBox's webhooks instead, so CI doesn't run the worker.

### Changed

- `make test-integration` tests only the packages that have integration
  tests, and CI's integration job runs after the unit tests, not beside
  them, so the two don't compete for the runner's memory.

### Fixed

- `nbpdns drift --zone` and `nbpdns netbox records --zone` find a NetBox
  zone whatever the case of its name. The DNS plugin keeps the case a
  zone's name was given in, so a zone named `Example.com` wasn't found.
- A release's notes on GitLab are its version's section of the CHANGELOG.
  The 0.1.0 release was published with empty notes.

### Security

- nbpdns is built with Go 1.27.2 and `golang.org/x/net` v0.60.0, which fix
  GO-2026-6617, a race in the HTTP/2 server's HPACK encoder that can crash
  it. `nbpdns serve` listens without TLS, where Go's server speaks only
  HTTP/1.1, so it wasn't exposed.

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
