---
id: ITEM-0103
title: Pin gopls with the other Go tools
type: debt # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-022, REQ-026]
depends_on: []
created: 2026-10-14
closed:
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

- [ ] `tools/gopls/go.mod` pins `gopls` with a `tool` directive, and
  `make tools-update` moves it to the latest release.
- [ ] One make target sets up the pinned `gopls` so that the plugin runs
  it, and a fresh checkout needs nothing else.
- [ ] `tools/gopls` is tidy, and ITEM-0102's check covers it.

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
