---
id: ITEM-0014
title: Pin tool release binaries instead of building from source
type: task
status: done
milestone: M00
requirements: [REQ-022, REQ-026, REQ-038]
depends_on: []
created: 2026-09-25
closed: 2026-09-25
---

# ITEM-0014: Pin tool release binaries instead of building from source

## Goal

The first GitLab job was evicted when its node ran out of ephemeral storage.
Building the eight pinned tools from source needs about 6.5 GB and 3.5
minutes from cold. Switch the six tools that publish release binaries to
those binaries, pinned by SHA-256, so CI fits ordinary runner nodes
(ADR-0014).

## Acceptance criteria

- [x] `tools/tools.mk` pins golangci-lint, Hugo, Vale, vacuum, gitleaks and htmltest for `linux-amd64`: repository, version, asset, SHA-256 and archive member.
- [x] `tools/fetch.sh` downloads, verifies against the pinned hash, and installs atomically. A wrong hash fails and leaves no binary behind.
- [x] Make targets depend on the binaries they use. An unpinned platform gets a message pointing to `make shell`.
- [x] `make tools-update` (`tools/update.sh`) and `make tools-audit` (`tools/update.sh --current`). With the current versions, the audit gives no diff.
- [x] govulncheck and goimports stay source-built. The six per-tool Go modules for binary tools are removed.
- [x] The C compiler prerequisite is dropped from `CLAUDE.md`, the README and the Makefile help.
- [x] ADR-0014 (supersedes ADR-0013), ADR-0015 (glibc images), REQ-038, and the brief updated.
- [x] `make ci` is green.

## Notes

- 2026-09-25: Measured the cost of building each tool on its own from empty
  caches, excluding the Go toolchain download (CI's image already has it).
  The totals combine downloaded source, unpacked source and build cache.

  | Tool | Build from source | Release binary download |
  |---|---|---|
  | vacuum | 3.4 GB | 19.2 MB |
  | Hugo | 1.3 GB | 20.0 MB |
  | golangci-lint | 0.9 GB | 14.8 MB |
  | Vale | 0.8 GB | 11.7 MB |
  | gitleaks | 0.6 GB | 7.8 MB |
  | htmltest | 155 MB | 2.2 MB |
  | govulncheck | 168 MB | none published |
  | goimports | 104 MB | none published |

  vacuum's cost comes from `modernc.org/sqlite`, SQLite machine-translated
  into Go. Vale's comes from tree-sitter grammars in C, built with cgo.
- 2026-09-25: Every pinned hash was checked against its release's checksums
  file, and against a local `sha256sum` of the download. All six matched.
- 2026-09-25: Vale's Linux release binary is dynamically linked against glibc.
  It runs on Debian and Ubuntu, but not on Alpine; that's accepted in
  ADR-0015.
- 2026-09-25: Results:
  - `make tools` fetched all six binaries in 8 seconds (230 MB on disk).
  - `make ci` then took 6 seconds with caches warm.
  - A wrong `HTMLTEST_SHA256_linux-amd64` failed with "SHA-256 mismatch",
    leaving no binary and no `.partial` file.
  - `PLATFORM=linux-arm64` gave "no pinned golangci-lint for linux-arm64; run
    the tools in the CI image with 'make shell'".
- 2026-09-25: `MEMBER` is per platform (`_MEMBER_linux-amd64`) rather than one
  value as planned, because golangci-lint's archive path includes the
  platform.
