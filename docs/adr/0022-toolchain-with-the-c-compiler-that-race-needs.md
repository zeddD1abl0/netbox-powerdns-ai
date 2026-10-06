---
title: "0022: Toolchain with the C compiler that -race needs"
status: accepted
date: 2026-09-29
decision-makers: [jordan]
requirements: [REQ-022, REQ-026, REQ-038]
questions: [Q-004]
supersedes: ADR-0014
---

# 0022: Toolchain with the C compiler that -race needs

## Context and problem statement

ADR-0014 pinned the development tools as release binaries, and listed the
prerequisites as Go, Docker, make, curl, tar and sha256sum. It said the C
compiler that ADR-0013 required was no longer needed.

That was wrong. `CLAUDE.md` runs every test with `-race`, and the race
detector needs cgo, so it needs a C compiler. On a host without one, Go
turns cgo off and `make test` fails with `go: -race requires cgo`. CI never
showed it, because the official golang image includes gcc. The review before
M01 found it, and ITEM-0025 reproduced it.

Since ADR-0014, the tool manifest has also grown: `jq` is pinned for the
hook tests (ITEM-0017). Its release tag isn't `v<version>`, and its asset is
the binary itself, not an archive.

Accepted ADRs aren't edited, so this ADR restates ADR-0014 with both
corrections. **The decision itself is unchanged.**

## Decision drivers

As in ADR-0014:
- CI must fit on ordinary runner nodes, and start quickly.
- Pins must stay exact and verifiable, in the repository.
- Keep it simple: no builds for several C libraries or platforms.

## Considered options

As in ADR-0014, for the tools:
1. Build every tool from source.
2. Release binaries pinned by SHA-256 where published; source only where
   not.
3. A prebuilt CI image containing the tools.

For the race detector:
1. Add a C compiler to the prerequisites.
2. Run the tests without `-race` on hosts that have no C compiler.

## Decision outcome

Chosen option: **release binaries pinned by SHA-256, source only where no
binary exists**, as in ADR-0014; and **a C compiler is a prerequisite**,
because `-race` is required everywhere and needs one.

- **Prerequisites.** Development and CI run on glibc Linux amd64, meaning
  Debian or Ubuntu. The prerequisites are Go, Docker, make, curl, tar,
  sha256sum, and a C compiler for cgo: `gcc` and `libc6-dev` on Debian or
  Ubuntu. On any other platform, `make shell` runs everything inside the
  pinned CI image, which has them all.
- **A clear failure.** Without cgo, `make test` and `make test-integration`
  stop before running any test, with a message that names the missing C
  compiler and how to install it.
- **Manifest.** `tools/tools.mk` gives each binary tool its repository and
  version, and per platform its asset, the SHA-256 the download must match,
  and the binary's path inside the archive, or `-` when the asset is the
  binary itself. The release tag is `v<version>`, unless the tool sets its
  own `_TAG`, as `jq` does. Only `linux-amd64` is pinned.
- **Fetching.** `tools/fetch.sh` downloads each binary once into
  `.cache/tools/<platform>/<name>-<version>`, and checks it against the
  **pinned** hash, never a checksums file fetched at build time. On a
  mismatch it fails and leaves no binary behind.
- **Updating.** `make tools-update` moves every tool to its latest release,
  taking hashes from each release's checksums file. `make tools-audit`
  re-derives the current pins from those files and fails on any difference.
- **Source-built tools.** govulncheck and goimports publish no binaries, and
  stay pinned through `tools/<name>/go.mod`.
- **Carried over unchanged:** anything else that isn't Go runs from a
  container image pinned by digest; `make` is the only entry point; CI files
  only run make targets (ADR-0016); UI and API-docs assets are vendored.

### Consequences

- Good: the prerequisites are now true, and a host without a C compiler
  gets a clear message instead of Go's.
- Good: as in ADR-0014, the binaries download in seconds, with nothing
  compiled for tools.
- Bad: one more prerequisite. Debian and Ubuntu hosts install it with
  `apt install gcc libc6-dev`, and other hosts use `make shell`.
- Bad: as in ADR-0014, the binaries depend on each project's release
  process, and each new platform needs one more hash per tool.

### Confirmation

- On a `PATH` without gcc, `make test` stops with the message, and with gcc
  `make check` passes (ITEM-0025).
- `make tools-audit` checks every pin against its published checksums.
- The CI image is Debian-based and pinned by digest (ADR-0015).

## Pros and cons of the options

### Tests without `-race` where there's no C compiler

- Good: one prerequisite fewer.
- Bad: data races would pass locally and fail only in CI, and `CLAUDE.md`
  requires `-race` for every test run.

## More information

- Supersedes [ADR-0014](0014-toolchain-pinned-release-binaries-on-glibc-linux.md),
  which superseded ADR-0013.
- The fault and its reproduction: ITEM-0025.
