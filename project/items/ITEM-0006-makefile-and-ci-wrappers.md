---
id: ITEM-0006
title: Makefile and thin CI wrappers for GitLab and GitHub
type: task
status: done
milestone: M00
requirements: [REQ-026]
depends_on: [ITEM-0004]
created: 2026-09-25
closed: 2026-09-25
---

# ITEM-0006: Makefile and thin CI wrappers for GitLab and GitHub

## Goal

Make `make` the single entry point, so CI on any forge, and any developer, runs
exactly the same steps (ADR-0003).

## Acceptance criteria

- [x] Targets: `help` (the default), `check`, `ci`, `fmt`, `vet`, `lint`, `test`, `test-integration`, `vuln`, `secrets`, `docs-lint`, `api-lint`, `project`, `project-lint`, `item`, `adr`, `tools-update` and `vale-sync`. The docs site targets come with ITEM-0008. `build`, `image` and `release-dry` wait for M1, when there's a binary to build.
- [x] `make help` lists every target with a one-line description, grouped.
- [x] `.gitlab-ci.yml` runs only `make ci`, in the official Go image pinned by digest, on the existing runners (Q-049).
- [x] `.github/workflows/ci.yml` runs the same `make ci` in the same pinned image. `actions/checkout` is pinned by commit SHA.
- [x] `make project-lint` fails if either CI file runs anything other than make targets (ITEM-0005).
- [x] `CLAUDE.md` command table updated to match.

## Notes

- 2026-09-25: Formatting is checked by `make lint`, because golangci-lint v2
  runs the configured formatters as part of `run`. That makes a separate
  `fmt-check` target unnecessary; `make fmt` rewrites files.
- 2026-09-25: `each_module` runs Go targets in every module that has packages.
  The root module has none until M1 and is skipped with a message, since
  `go vet` fails outright on an empty module.
- 2026-09-25: Both CI files use `GOTOOLCHAIN=local`, so the image must match
  `go.mod`'s toolchain; CI never downloads a different Go. GitHub's container
  job sets `safe.directory` through `GIT_CONFIG_*` environment variables
  rather than a `git config` command, which the make-only rule would reject.
- 2026-09-25: Verified by running `make ci` in the pinned image
  (`golang:1.27.1@sha256:3680233e…`) as a non-root user with cold caches. The
  first run exposed a bug: `docs-lint` counted `go tool`'s download lines and
  Vale's cgo warning, printed on stderr, as findings. Fixed by capturing only
  stdout and checking Vale's exit code separately. The second run passed in
  1m49s, and a deliberately banned word still fails the target.
