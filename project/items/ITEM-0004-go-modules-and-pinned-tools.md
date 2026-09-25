---
id: ITEM-0004
title: Go module, pinned tools module and license
type: task
status: blocked
milestone: M00
requirements: [REQ-001, REQ-026]
depends_on: [Q-055]
created: 2026-09-25
closed:
---

# ITEM-0004: Go module, pinned tools module and license

## Goal

Create the root `go.mod` with the agreed module path, and a separate
`tools/go.mod` that pins every Go-based tool through `tool` directives. Keeping
tools in their own module keeps their dependencies out of the product's
`go.mod`. Add the `LICENSE`.

## Acceptance criteria

- [ ] `go.mod` uses the module path `github.com/<owner>/netbox-powerdns-ai` (ADR-0005; owner from Q-055), and a `toolchain` line pins the Go version.
- [ ] `tools/go.mod` pins golangci-lint, govulncheck, vacuum, Vale, oapi-codegen, goreleaser and gitleaks. Hugo is pinned per ITEM-0008.
- [ ] `go tool -modfile=tools/go.mod <tool> --version` works for each pinned tool.
- [x] `LICENSE` holds the Apache-2.0 text (Q-005).
- [ ] The `.claude/settings.json` hook switches from `gofmt` to pinned `goimports`.

## Notes

- 2026-09-25: Blocked on Q-005. The local Go is 1.24.10, and go-redbarkwebhook's
  CI uses 1.26. The `toolchain` directive will fetch the pinned version
  automatically.
- 2026-09-25: `LICENSE` added from `/usr/share/common-licenses/Apache-2.0`.
  Its SHA-256 `cfc7749b96f63bd31c3c42b5c471bf756814053e847c10f3eb003417bc523d30`
  matches the canonical Apache-2.0 text. Still blocked on Q-055 for `go.mod`.
