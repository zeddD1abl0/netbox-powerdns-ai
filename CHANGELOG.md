# Changelog

All notable changes to this project are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- `nbpdns netbox check`, `zones`, and `records`: read DNS data from the NetBox
  DNS plugin, through NetBox's REST API, with a read-only token. `check`
  reports NetBox's and the plugin's releases, and whether the token can view
  each kind of object. `records` shows a zone in nbpdns's normalized form:
  RRsets with absolute names, canonical values, and one TTL each, with
  inactive records listed. It reports problems in NetBox's data as warnings,
  and in its JSON output, without failing. A zone name in more than one view
  needs `--view`.
- PowerDNS server groups, declared in the config file under
  `powerdns.groups`: each a name, the NetBox views it serves, and its
  primary's API URL, API key or key file, server ID, CA file, and client
  certificate. Fields are checked strictly, every problem is reported at
  once, and `nbpdns config show` lists each field with the key redacted.
  `powerdns.timeout` and `powerdns.concurrency` set how requests to PowerDNS
  behave.
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
  and are redacted everywhere they could be printed or logged. Unknown keys and
  variables are errors, and every problem is reported at once.
- Configuration and command-line reference pages, generated from the code.
- A development lab with NetBox 4.7 and the NetBox DNS plugin, in containers
  (`make lab-up`, `make lab-down`). Integration tests run against it
  (`make test-integration`), in every pipeline.
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
