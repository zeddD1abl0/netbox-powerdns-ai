# Changelog

All notable changes to this project are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

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
