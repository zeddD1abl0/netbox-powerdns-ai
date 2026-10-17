---
id: ITEM-0103
title: Pin gopls with the other Go tools
type: debt # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-022, REQ-026]
depends_on: []
created: 2026-10-14
closed: 2026-10-16
---

# ITEM-0103: Pin gopls with the other Go tools

## Goal

The `gopls-lsp` plugin gives Claude code intelligence through `gopls`,
which it runs from `PATH`. The one it finds, `~/go/bin/gopls` v0.23.0, was
installed by hand at no pinned version, unlike every other tool
(ADR-0013, ADR-0022). A `gopls` that drifts from the Go toolchain gives
wrong or missing diagnostics, and another machine would get a different
one. Pin it the way the other Go tools are pinned, and make the plugin
run the pinned build.

## Acceptance criteria

- [x] `tools/gopls/go.mod` pins `gopls` with a `tool` directive, and
  `make tools-update` moves it to the latest release.
- [x] One make target sets up the pinned `gopls` so that the plugin runs
  it, and a fresh checkout needs nothing else.
- [x] `tools/gopls` is tidy, and ITEM-0102's check covers it.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-14: Found when the plugin was installed: `gopls` came from
  `go install golang.org/x/tools/gopls@latest`, and `git grep gopls`
  finds nothing in the tracked files. How the Go tools are pinned now,
  found by `scout` and checked against the files: Makefile line 41
  defines `tool = go tool -modfile=$(ROOT)/tools/$(1)/go.mod $(1)`;
  `tools/goimports`, `tools/govulncheck` and `tools/oapi-codegen` each
  have a `tool` directive on line 7 of their `go.mod`; and
  `tools-update` (Makefile lines 47 to 55) runs `go get -tool …@latest`
  and `go mod tidy` for each tool in a list written into the target, so
  `gopls` must join that list. The plugin runs the command `gopls`, as
  its `lspServers` entry in the official marketplace says, so the item
  must choose how that resolves to the pinned build: a make target that
  builds it into `GOBIN`, or a wrapper on a `PATH` that Claude Code's
  environment sets. `gopls` is in `golang.org/x/tools`, under the
  BSD-3-Clause license.
- 2026-10-16: Done. Which `PATH` the plugin's `gopls` sees: Claude Code
  starts it as its own child, in the repository's directory, with its own
  environment, whose `PATH` VS Code passed on from the login shell. It
  found `~/go/bin/gopls`, v0.23.0 built with go1.26.5. Claude Code's
  settings can't extend that `PATH`: their docs say an `env` block's values
  are copied as written, with no `$PATH` or `~` expansion, so a checked-in
  settings file could only replace the whole `PATH` with one machine's
  paths. That rules out a wrapper. A third option, a plugin in the
  repository whose LSP server runs
  `go tool -modfile=${CLAUDE_PROJECT_DIR}/tools/gopls/go.mod gopls`, with
  the official plugin turned off for the project, was set aside: it needs
  a local marketplace, the folder trusted and the plugin enabled, and it
  compiles `gopls` on its first start. Chosen, with the user: `make gopls`
  runs `go -C tools/gopls install tool`, into `GOBIN` or the first
  `GOPATH`'s `bin`, and fails unless `command -v gopls` is the file it
  installed. The user agreed that it replaces `~/go/bin/gopls`, which
  every project shares, and asked that `make tools-update` end by running
  `make gopls`, so an update never leaves the old build in use. No ADR:
  ADR-0022 already pins source-built tools through `tools/<name>/go.mod`,
  and only where this one's binary goes is new.
  The dependency: `gopls` v0.24.0 (2026-10-14, needs Go 1.27.0), with
  `toolchain go1.27.2`, so it's built with the repository's toolchain. It
  is the Go team's language server, the one the official plugin runs,
  under BSD-3-Clause, and nbpdns's binary never includes it. Its 24
  indirect modules are under BSD-3-Clause, MIT or Apache-2.0 (the MCP SDK
  is moving from MIT to Apache-2.0), except `github.com/segmentio/asm`,
  linked in through the MCP SDK, under MIT-0: MIT without its attribution
  condition.
  Checked: `make gopls` installed v0.24.0, built with go1.27.2, at
  `~/go/bin/gopls`. With another `gopls` earlier on `PATH`, and with
  `GOBIN` off `PATH`, it failed and named the directory. After the
  plugin's old `gopls` was stopped, an LSP hover started a new one from
  the pinned binary. `make -n tools-update` lists `gopls` and ends with
  `make gopls`. In the CI image, with `GOTOOLCHAIN=local` and an empty
  module cache, `tools/gopls` is tidy, and after the other modules'
  `tidy -diff` it adds 92 MB and 4 s to `go-lint` (149 MB cold, alone).
  The README's Development section and the CHANGELOG, under Added, each
  gained a paragraph. `make check` passed.
