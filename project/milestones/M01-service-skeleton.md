---
id: M01
title: Service skeleton
status: in-progress # planned | in-progress | done
started: 2026-09-26
closed:
---

# M01: Service skeleton

> [!IMPORTANT]
> **M01 has been re-planned as "NetBox read path".** The approved design below
> replaces this stub's goal and scope. ITEM-0019, the first implementation
> step after ITEM-0018, rewrites and renames this file and the other stubs.
>
> **State for the next session (2026-09-27):**
> - The design was approved, with implementation deferred.
> - Items ITEM-0018 to ITEM-0024 are filed and open. ITEM-0017 is also
>   open.
> - Nothing is implemented yet.
> - **Start with** ITEM-0018 (the merge-request process), then ITEM-0019
>   (the re-slice, which renames this branch to `m01-netbox-read-path`).
> - **On the user's side:** set GitLab's merge method to Merge commit, with
>   squash disallowed, and size the runners for the Docker-in-Docker
>   integration job.

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

## Verification log

Append-only and dated. Record what was run and what was seen.

## Approved design

The plan approved on 2026-09-27, copied verbatim. Its headings are demoted two
levels to nest under this section; the text is unchanged. It's a snapshot.
Where it disagrees with an ADR or `CLAUDE.md`, they win.

### Smaller milestones, the merge-request process, and M01: NetBox read path

> **Status, 2026-09-27:** approved by the user, with implementation deferred.
> Leaving plan mode now only records this plan in the repository: under
> **Approved design** in M01's milestone file, plus a hand-off note, in one
> commit on the M01 branch. Nothing below is implemented until the user says
> to start.

#### Context

M00 is merged: GitLab fast-forwarded `main` to `fcbdaeb`, and GitHub gets it
through a push mirror. The M01 design session produced these decisions from
the user:

- **Smaller milestones.** Each merge should be a useful, logical step, not a
  large one. M01 reads from the NetBox API, then later milestones add
  PowerDNS, then the database (SQLite, then PostgreSQL).
- **Unauthenticated first, then authentication.** Read-only features come
  first. Authentication arrives before anything can change state from outside.
- **Merges** go through a GitLab merge request with a merge commit. GitHub is a
  push mirror.
- **Libraries instead of custom modules:** Cobra for the command line and
  Viper for configuration. For logging, the user compared slog with zap and
  chose the stdlib's `log/slog`.
- **NetBox webhooks are needed.** Where they go was left to Claude: they get
  their own milestone, M06, after the REST API exists.

Answers given in the session:
- **Q-007:** support NetBox 4.7 and 4.6, with the DNS plugin 1.7.x and 1.6.x.
  NetBox 4.7.1 is current (2026-09-15), and 4.6.10 is the previous minor.
- **CI:** integration tests against a real NetBox run in every pipeline, using
  Docker-in-Docker.

This plan does three things, in this order:
1. It re-slices the milestone list.
2. It records the merge-request process.
3. It designs the new M01.

Everything happens on the M01 branch. That branch is renamed from
`m01-service-skeleton` to `m01-netbox-read-path` (it's local only, one commit).

#### 1. Smaller milestones (ITEM-0019, ADR-0019)

The new provisional list is below. Each milestone is still designed in its
own plan-mode session, which may split it further.

| M | Title | Delivers |
|---|---|---|
| M01 | NetBox read path | The `nbpdns` binary, config, logging, the NetBox DNS plugin client, the NetBox lab, and the `nbpdns netbox …` commands |
| M02 | PowerDNS read path | The PowerDNS API client, server groups declared in the config file (ADR-0007), a PowerDNS lab (two groups, each a primary and a secondary), `nbpdns powerdns …` |
| M03 | Drift report | Compare NetBox with PowerDNS for each group, in memory. Text and JSON reports, and exit codes for scripts. Read-only. |
| M04 | Service | `nbpdns serve` runs continuously and refreshes drift on a schedule. It adds health and readiness, Prometheus metrics, OTLP trace export, the container image and GoReleaser. |
| M05 | REST API | The OpenAPI 3.1 pipeline (spec, oapi-codegen, contract tests, the reference served at `/api/docs`) and read-only, unauthenticated drift endpoints |
| M06 | NetBox webhooks | Receive NetBox event-rule webhooks, check their HMAC signature, refresh the affected zones, and carry NetBox's request and user into the trace |
| M07 | SQLite persistence | Migrations, the audit core, runtime settings, drift history, the single-instance lock, secrets encrypted at rest |
| M08 | PostgreSQL and HA | The second dialect, leader election, replica tests |
| M09 | Authentication | The break-glass token, local users with MFA, API tokens and service accounts, sessions. The API is locked down. |
| M10 | SSO and RBAC | OIDC, SAML, proxy headers, roles, the permission reference |
| M11 | Web UI | templ + htmx: login, dashboard, drift, settings, audit viewer |
| M12 | Write path: plan and apply | Plans, applying to primaries, catalog zones, the `enforce` policy |
| M13 | Change safety | Change limits, approvals, verification after apply |
| M14 | Brownfield import | Import existing PowerDNS zones into NetBox |
| M15 | SIEM export | Sinks, the outbox, hash-chain verification, the event catalogue |
| M16 | Production hardening | Helm, systemd, backup and restore, export and import, threat model, load test |
| M17 | Terraform/OpenTofu provider | A separate repository |
| M18 | Ansible collection and v1.0 | A separate repository, then the v1.0 release |

**Mechanics:**
- **Existing stubs.** M01 to M08 were never worked on, so they're rewritten to
  the table above, and renamed where the title changed. For example,
  `M01-service-skeleton.md` becomes `M01-netbox-read-path.md`. New stubs are
  created for M09 to M18.
- **ADR-0019** records the re-slice. It includes a table mapping old
  milestones to new ones, so references like "M3 design" in accepted ADRs
  (ADR-0006, 0008, 0009, 0012, 0015) can still be resolved.
- **`CLAUDE.md`** gets this rule: "Files never move or get renamed. The one
  exception: a planned milestone stub with no work yet may be rewritten or
  renamed by a re-plan recorded in an ADR." `CLAUDE.md` is at its 150-line
  limit, so an existing line is tightened to make room.
- **`requirements.md`.** Each open question's "Needed by" moves to its new
  milestone:

  | Needed by | Questions |
  |---|---|
  | M02 | Q-021, Q-022, Q-039, Q-043, Q-053 |
  | M03 | Q-017, Q-027 |
  | M04 | Q-025 (binary, image; the rest in M16), Q-038 |
  | M05 | Q-041 |
  | M06 | Q-037, Q-054 (triggers; import in M14) |
  | M07 | Q-012, Q-023, Q-024, Q-026 (migrations; the rest in M16), Q-035, Q-036 |
  | M09 | Q-029, Q-031, Q-040 |
  | M10 | Q-015, Q-028, Q-030, Q-032 |
  | M11 | Q-016, Q-050, Q-051 |
  | M12 | Q-011 |
  | M13 | Q-013, Q-014 |
  | M15 | Q-033, Q-034 |
  | M17 | Q-042 |
- **Docs.** The milestone numbers in `docs/tutorials/_index.md`,
  `docs/explanation/_index.md` and `docs/reference/_index.md` are updated.

#### 2. The merge-request process (ITEM-0018, ADR-0018)

- **ADR-0018** records how milestones merge:
  - GitLab is the primary forge. Each milestone merges through a merge request
    with the **Merge commit** method; squash and fast-forward are not used.
  - The merge commit is the milestone's revert point (ADR-0010).
  - GitHub is a push mirror of every branch, configured in GitLab.
  - If GitLab isn't available, the fallback is `git merge --no-ff` locally,
    then pushing `main` to both remotes.
  - It notes that M00 was fast-forwarded, before this rule existed.
- **`CLAUDE.md`:** the commits bullet now says the user merges through a
  GitLab MR with a merge commit (ADR-0018).
- **The `close-milestone` skill:** step 8 now also produces the MR title and a
  paste-ready description. It uses the M00 sections: summary, what's included,
  verification, reviewing, known and deferred, merging. Paths are shown as
  code, not links.

#### 3. M01: NetBox read path

##### Goal

A runnable `nbpdns` binary that reads DNS data from NetBox's DNS plugin and
shows it. It lays the command-line, config, logging and tracing foundations
every later milestone uses, and the Docker-based lab and CI that integration
tests need.

##### Non-goals

- No PowerDNS (M02), no comparison (M03), no long-running service (M04), no
  REST API (M05), no webhooks (M06).
- No database or state (M07).
- No authentication of nbpdns's own users (M09). M01 only authenticates *to*
  NetBox.
- No writes to NetBox.

##### Decisions

- **ADR-0020, the NetBox client:**
  - It uses the **REST API only**. ADR-0006 left GraphQL to evaluate; REST is
    stable, documented and filterable, and paging it scales to the default
    targets.
  - **Supported versions:** NetBox 4.7 and 4.6, with plugin 1.7.x and 1.6.x.
  - It authenticates with a **read-only v2 token** (`Authorization: Bearer
    nbt_…`). v1 tokens still work, with a warning that recommends v2.
  - Q-007 moves to Answered, producing **REQ-040**.
- **ADR-0021, Cobra and Viper** for the command line and configuration, chosen
  by the user over stdlib-only modules.
  - Cobra (Apache-2.0) gives subcommands, help, shell completion, and a
    generated command reference (`cobra/doc`).
  - Viper (MIT) gives config-file loading and its precedence: flags > env >
    file > defaults.
  - The ADR records the dependency cost, and what nbpdns adds on top (below).
- **Logging stays `log/slog`**, as `CLAUDE.md` says. The user compared slog with
  zap and chose slog: no dependency, the context reaches the handler (for
  trace IDs), and secrets redact themselves through `LogValuer`. If needed
  later, zap can run behind the slog API (`zapslog`).
- **Config file format:** YAML, which Viper reads.
- **Tracing:** the OpenTelemetry API and SDK, so every run has a trace ID and
  outbound NetBox calls carry W3C `traceparent`. There's no exporter until M04.
  The standard `OTEL_*` variables aren't a supported interface; nbpdns has its
  own keys.

**New dependencies:**
- `github.com/spf13/cobra` and `github.com/spf13/viper`, and their transitive
  modules;
- `go.opentelemetry.io/otel` and `go.opentelemetry.io/otel/sdk`.

ITEM-0020 lists every module added to `go.mod`, with its license, which must be
MIT, BSD, Apache-2.0 or MPL-2.0.

##### Design

**Layout:**
- `cmd/nbpdns` holds the binary.
- `internal/` holds:
  - `cli`: the Cobra commands;
  - `config`: the key registry, Viper wiring and the secret type;
  - `logging`;
  - `tracing`;
  - `netbox`: the client;
  - `dns`: the normalized model shared by M02 and M03;
  - `version`.
- `internal/cmd/gendocs` generates the reference pages. It stays out of the
  binary.

**Commands** (Cobra). Logs go to stderr and output to stdout. Read commands
accept `--output table|json`.

| Command | Does |
|---|---|
| `nbpdns version` | Version, commit and Go version, from `debug.ReadBuildInfo` |
| `nbpdns config show` | Every key's effective value and its source. Secrets show as `[redacted]`. |
| `nbpdns netbox check` | Reports the NetBox and plugin versions and whether they're supported, and confirms the token can read views, zones and records. Exits non-zero, with a message saying what's wrong. |
| `nbpdns netbox zones [--view V] [--status S]` | Lists zones: view, status, SOA serial, default TTL, nameservers |
| `nbpdns netbox records --zone Z [--view V]` | Lists a zone's records in the normalized model |
| `nbpdns completion …` | Cobra's shell completion |

**Configuration** (`internal/config`): Viper does the loading, and a thin key
registry keeps the "declared once" rule.
- **The registry.** Each key is declared once, with its name, type, default,
  description and a secret flag. That declaration drives Viper's defaults,
  the Cobra flag, the env binding, and the generated reference.
  - From `netbox.url` it derives `NBPDNS_NETBOX_URL` (Viper's env prefix and
    key replacer), `--netbox-url`, and the file path `netbox: {url: …}`.
  - A runtime flag is reserved for M07.
- **What the registry adds, because Viper doesn't do it:**
  - **Secret files.** A secret key also gets `_FILE` variants
    (`NBPDNS_NETBOX_TOKEN_FILE`, `--netbox-token-file`), resolved at the same
    precedence level as their plain form. Setting both forms of one key is an
    error.
  - **Strict keys.** An unknown file key is caught with `UnmarshalExact` into
    the typed config. An unknown `NBPDNS_*` variable is caught by checking the
    environment against the registry. Both are errors, and all validation
    errors are reported together.
  - **Where a value came from**, for `config show`: a flag that was changed,
    an env variable that is set, `InConfig` for the file, or else the
    default.
- **Precedence:** defaults < file (`--config` or `NBPDNS_CONFIG`) < env <
  flags.
- **Redaction.** `config.Secret` redacts itself in `String`, `Format`,
  `MarshalJSON` and slog's `LogValue`. Only an explicit `Reveal()` exposes the
  value.
- **M01 keys:**
  - `log.level`, and `log.format` (`json` or `text`);
  - `netbox.url` and `netbox.token` (a secret);
  - `netbox.ca_file`, `netbox.timeout`, `netbox.page_size` and
    `netbox.concurrency`.
- **Generated references:**
  - `docs/reference/configuration.md`, from the registry;
  - the command-line reference, from Cobra's command tree through
    `cobra/doc`, with Hugo front matter and without the auto-generated tag.

  `make generate` writes them. `make generate-check` regenerates them into a
  temporary directory and fails on any difference.

**Logging and tracing:**
- Logs use `slog`, JSON by default.
- A handler adds `trace_id` and `span_id` from the context, and a
  `request_id`: for a command run, that's the invocation's ID; in M04 it
  becomes the HTTP request's ID.
- Each command runs in a root span.
- The NetBox transport injects `traceparent` through the OTel propagator,
  without the otelhttp dependency.

**NetBox client** (`internal/netbox`):
- `/api/status/` gives the NetBox version and the installed plugins, which the
  client checks against REQ-040.
- It reads the plugin's `views`, `zones`, `nameservers` and `records`
  endpoints:
  - it follows the `next` links, with `limit` set to `netbox.page_size`;
  - it fetches records per zone (`zone_id`), with at most
    `netbox.concurrency` requests in flight.
- **Timeouts and retries.** Each request has a timeout. GETs are retried on
  network errors, 429, 502, 503 and 504, with capped exponential backoff and
  jitter, honoring `Retry-After`.
- **TLS:** the system roots plus an optional `netbox.ca_file`, TLS 1.2 or
  later. There's no option to skip verification.
- **Errors are typed:** unreachable, authentication failed, missing permission
  (naming the object type), unsupported version, and API errors with NetBox's
  own detail.

**Normalized model** (`internal/dns`). The rules are documented and tested
here, because M02 maps PowerDNS into the same model and M03 compares the two.
- Views, zones and records.
- Names are lowercase, absolute FQDNs with a trailing dot.
- Relative targets in CNAME, MX, NS, SRV and PTR records are made absolute.
- TXT values are in canonical quoted form.
- A record's TTL is its effective TTL: its own, or the zone's default.
- Each record keeps its status, and a `managed` flag for records the plugin
  generates (SOA, NS, PTR).

**Lab** (`deploy/dev/`):
- **`compose.yaml`** runs:
  - PostgreSQL, with one database per NetBox;
  - Valkey;
  - NetBox 4.7 with plugin 1.7.x on port 8047;
  - NetBox 4.6 with plugin 1.6.x on port 8046.
- **Images.** Both NetBox images are built from `deploy/dev/netbox/Dockerfile`
  on the official netbox-docker image. Every image is pinned by digest, and
  the plugin by version.
- **Make targets:**
  - `make lab-up` builds and starts the lab and waits for health checks;
  - `make lab-down` removes it, with its volumes;
  - `make test-integration` depends on `lab-up`.
- **docker-compose** is pinned in `tools/tools.mk` as a release binary.
  - `tools/fetch.sh` learns to fetch an asset that is itself the binary (a
    member of `-`).
  - If compose can't build without the buildx plugin, buildx is pinned the
    same way.
- **The lab host** comes from `DOCKER_HOST` (for example `tcp://docker:2375`
  gives `docker`), or is `localhost`.

**Integration tests** (`//go:build integration`):
- They run once against each NetBox version.
- The admin account creates the fixtures: a view, zones, and more than one
  page of records. The client then reads them back with a least-privilege
  user whose v2 token can only view netbox_dns objects.
- Failure cases:
  - a token without permission gets a clear error;
  - an unsupported version gets a warning.
- **Unit tests** cover the normalization rules and the mapping of responses
  recorded from the lab (`testdata/`). No hand-written fake of NetBox's API.

**Make and CI:**
- New targets: `build` (a static binary in `bin/`, CGO off), `generate`,
  `generate-check`, `lab-up` and `lab-down`.
- `make check` gains `generate-check`.
- `make ci` becomes `check`, `build`, `docs-links` and `test-integration`.
- New CI jobs on both forges:
  - `build`, in the build stage;
  - `integration-test`, in the test stage, with a digest-pinned
    `docker:dind` service. On GitHub that's a privileged service container.
    `DOCKER_HOST` points at it.
- `make project-lint` keeps both forges in step, as today.

**Docs:**

| Section | Pages |
|---|---|
| Tutorial | "Read your NetBox DNS data with nbpdns", using the lab |
| How-to | "Give nbpdns read-only access to NetBox" (user, permission, v2 token); "Configure nbpdns" (file, env, flags, secret files); "Run the development lab" |
| Reference | Configuration and command line (both generated); "Supported versions" |
| Explanation | "How nbpdns reads NetBox" (model, normalization, paging, why REST); "Configuration sources and precedence" |

Plus `CHANGELOG.md` lines under Unreleased.

##### Items and phases

| Phase | Items |
|---|---|
| M1a Process | ITEM-0018 merge-request process; ITEM-0019 re-slice the milestones |
| M1b Skeleton | ITEM-0020 Cobra commands, the Viper config registry and generated references (ADR-0021); ITEM-0021 logging and tracing; ITEM-0017 hook tests in `make test` (already filed) |
| M1c Lab and CI | ITEM-0022 NetBox lab, compose pin, Docker-in-Docker integration job on both forges |
| M1d NetBox | ITEM-0023 NetBox client and normalized model (ADR-0020, REQ-040); ITEM-0024 `nbpdns netbox` commands, docs, CHANGELOG |

##### Acceptance criteria

- [ ] ADR-0018 and ADR-0019 are accepted. The stubs M01 to M18 match the new
  list, `requirements.md` is remapped, and the `close-milestone` skill writes
  MR descriptions.
- [ ] `nbpdns` builds as a static binary. `version`, `config show`,
  `completion` and the `netbox` commands work as designed.
- [ ] Config precedence, `_FILE` secrets, strict unknown keys (file and env),
  source reporting and redaction are covered by table-driven tests.
  `make generate-check` fails on a stale reference.
- [ ] Every log line carries `trace_id` and `request_id`, and NetBox requests
  carry `traceparent`.
- [ ] `make lab-up` starts NetBox 4.7 and 4.6 with the plugin. The integration
  tests pass against both, with a least-privilege v2 token.
- [ ] Integration tests run in every GitLab and GitHub pipeline, and
  `make project-lint` confirms the CI files mirror `make ci`.
- [ ] Hook pipe-tests run in `make test` (ITEM-0017).
- [ ] The docs pages above exist, the generated references are current, and
  the CHANGELOG is updated.
- [ ] `/code-review high` and `/security-review` have run. M01 handles the
  NetBox token, so the secrets trigger applies.
- [ ] The manual verification is recorded, and the user has merged through an
  MR with a merge commit.

#### Verification

- `make ci` passes on every commit, including the integration tests against
  both NetBox versions.
- **Manual check:**
  1. Run `make lab-up`.
  2. Create a zone and records in NetBox 4.7's UI.
  3. `nbpdns netbox check` shows versions 4.7 and 1.7.x as supported.
  4. `nbpdns netbox zones` and `records` show the data, in table and JSON
     form.
  5. Repeat against 4.6.
  6. A wrong token, and a token without DNS permissions, each give a clear
     error.
  7. `config show` redacts the token, and shows which source each value came
     from.
- The first pipeline with the integration job passes on both forges. The
  job's peak ephemeral storage is measured and reported to you.

#### Your side

- **GitLab**, under Settings, then Merge requests: set the merge method to
  **Merge commit**, and squash to **Do not allow**.
- **GitLab runners:** the integration job needs Docker-in-Docker, with an
  estimated 4–6 GB of ephemeral storage. ITEM-0022 measures the real figure.
