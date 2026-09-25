---
title: "0013: Self-contained toolchain, revised — per-tool modules and a C compiler"
status: accepted
date: 2026-09-25
decision-makers: [jordan]
requirements: [REQ-022, REQ-026]
questions: [Q-004]
supersedes: ADR-0003
---

# 0013: Self-contained toolchain, revised — per-tool modules and a C compiler

## Context and problem statement

ADR-0003 made the project self-contained: prerequisites of Go, Docker and make
only, every tool pinned in the repo, and `make` as the only entry point.
Building the toolchain (ITEM-0004) showed that two of its details don't hold:

1. **"Go tools use `tool` directives in `tools/go.mod`."** With a single tools
   module, Go's minimum version selection merges every tool's dependencies. A
   tool could then build against versions its authors never tested.
   golangci-lint in particular warns against this.
2. **"The only prerequisites are Go, Docker and make."** Vale (v3.22) depends on
   `go-tree-sitter`, which needs cgo. It won't build with `CGO_ENABLED=0`. Every
   other pinned tool builds as pure Go.

This ADR restates ADR-0003 with those two points changed. Everything else in
ADR-0003 carries over unchanged.

## Decision drivers

- Unchanged from ADR-0003: moving forges must not lose knowledge or break the
  build; CI must be reproducible locally; no runtime CDNs.
- Each pinned tool should build exactly as its authors tested it.

## Considered options

For Vale's cgo requirement:
1. Accept a C compiler as a prerequisite.
2. Run Vale from a pinned container image. CI would then need Docker for doc
   linting.
3. Download Vale's release binary, pinned by SHA-256 per platform.
4. Drop Vale and check the style rules in `projctl`.

## Decision outcome

**Prerequisites:** Go, Docker, make, and **a C compiler** (for cgo).
- `gcc` ships in the official `golang` images, which CI uses, and in standard
  Linux build tooling.
- Today only Vale needs it. Any other tool that needs cgo must be noted in its
  item.

**Tools: one small module per tool.**
- Each pinned tool has its own module in `tools/<name>/go.mod`, with a single
  `tool` directive, so it builds with exactly its own dependency versions.
- Tools run from the repo root with `go tool -modfile=tools/<name>/go.mod <name>`.
  The Makefile wraps this, so nobody types it.
- In-house tooling (`projctl`) is its own module under `tools/projctl`, so its
  dependencies stay out of the product's `go.mod`.

**Carried over unchanged from ADR-0003:**
- anything that isn't Go runs from a container image pinned by digest;
- `make` is the only entry point, and `make ci` runs the whole pipeline
  locally;
- `.gitlab-ci.yml` and `.github/workflows/*.yml` only call make targets;
- all knowledge lives in the repo; no forge features are used as a record;
- UI and API-docs assets are vendored, with no runtime CDN;
- releases are built by a forge-neutral tool (GoReleaser is the candidate).

### Consequences

- Good: tool builds are reproducible and isolated from each other.
- Good: accepting a C compiler (option 1) keeps Vale fast and native, with no
  Docker needed for linting.
- Bad: one more prerequisite. On a machine without `gcc`, `make docs-lint`
  fails with a cgo error; the Makefile's help text names the requirement.
- Bad: eight small `go.mod`/`go.sum` pairs to update instead of one.
  `make tools-update` bumps them all.

### Confirmation

- `projctl lint` checks that CI files call only make targets (from ADR-0003).
- The CI image is the official `golang` image, pinned by digest, which
  includes `gcc`.

## More information

- Supersedes [ADR-0003](0003-self-contained-forge-neutral-toolchain.md).
- Found during ITEM-0004. The pinned versions are listed in the item's notes.
