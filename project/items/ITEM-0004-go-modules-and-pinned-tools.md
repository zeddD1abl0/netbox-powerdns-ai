---
id: ITEM-0004
title: Go module, pinned tools module and license
type: task
status: done
milestone: M00
requirements: [REQ-001, REQ-026]
depends_on: [Q-055]
created: 2026-09-25
closed: 2026-09-25
---

# ITEM-0004: Go module, pinned tools module and license

## Goal

Create the root `go.mod` with the agreed module path, and a separate
`tools/go.mod` that pins every Go-based tool through `tool` directives. Keeping
tools in their own module keeps their dependencies out of the product's
`go.mod`. Add the `LICENSE`.

## Acceptance criteria

- [x] `go.mod` uses the module path `github.com/zeddD1abl0/netbox-powerdns-ai` (ADR-0005), with `go 1.27.0` and `toolchain go1.27.1`.
- [x] One module per tool under `tools/<name>/` (ADR-0013) pins golangci-lint, govulncheck, goimports, vacuum, Vale, gitleaks, Hugo and htmltest. oapi-codegen and goreleaser wait until M1 needs them.
- [x] `go tool -modfile=tools/<name>/go.mod <name>` runs each pinned tool from the repo root.
- [x] `LICENSE` holds the Apache-2.0 text (Q-005).
- [x] The `.claude/settings.json` format hook uses the pinned goimports.

## Notes

- 2026-09-25: Blocked on Q-005. The local Go is 1.24.10, and go-redbarkwebhook's
  CI uses 1.26. The `toolchain` directive will fetch the pinned version
  automatically.
- 2026-09-25: `LICENSE` added from `/usr/share/common-licenses/Apache-2.0`.
  Its SHA-256 `cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30`
  matches the canonical Apache-2.0 text. Still blocked on Q-055 for `go.mod`.
- 2026-09-25: Unblocked. Q-055 answered: `github.com/zeddD1abl0/netbox-powerdns-ai`.
- 2026-09-25: Pinned versions: golangci-lint 2.14.0, govulncheck 1.8.0, Hugo
  0.166.0, vacuum 0.30.6, Vale 3.22.0, gitleaks 8.30.1, htmltest 0.17.0, and
  goimports from `golang.org/x/tools` (see `tools/goimports/go.mod`).
- 2026-09-25: Two deviations from the plan, recorded in ADR-0013, which
  supersedes ADR-0003:
  - **one module per tool**, not a single `tools/go.mod`, so each tool builds
    with exactly its own dependency versions;
  - **Vale needs cgo**: `go-tree-sitter` won't build with `CGO_ENABLED=0`. A C
    compiler is now a prerequisite. The `golang:1.27.1` image ships gcc, make
    and git. All other tools build without cgo.
- 2026-09-25: Vale moved from `github.com/errata-ai/vale/v3` to
  `github.com/vale-cli/vale/v3`; the new path is pinned.
- 2026-09-25: The goimports hook was pipe-tested and proven live: a scratch
  file with its imports in the wrong order was fixed straight after being
  written.
