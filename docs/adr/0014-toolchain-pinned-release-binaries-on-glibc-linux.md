---
title: "0014: Toolchain: pinned release binaries on glibc Linux"
status: accepted
date: 2026-09-25
decision-makers: [jordan]
requirements: [REQ-022, REQ-026, REQ-038]
questions: [Q-004]
supersedes: ADR-0013
---

# 0014: Toolchain: pinned release binaries on glibc Linux

## Context and problem statement

ADR-0013 pinned every tool as a Go module and built it from source. The first
GitLab pipeline was evicted when the node ran out of ephemeral storage.
Measured from cold, building the eight tools took about 6.5 GB of disk and 3.5
minutes, all for a repository of 6 MB:
- vacuum alone took 3.4 GB, compiling SQLite translated into Go;
- Hugo took 1.3 GB, golangci-lint 0.9 GB and Vale 0.8 GB.

Six of the eight tools publish release binaries, with checksum files, for
Linux amd64. Together they're a 76 MB download.

The user decided on 2026-09-25:
- glibc-based systems are fine (ADR-0015);
- tools are pinned for **Linux amd64 only** for now. arm64 may follow, and
  macOS or Windows later. Linux is the expected deployment.

## Decision drivers

- CI must fit on ordinary runner nodes, and start quickly.
- Pins must stay exact and verifiable, in the repository.
- Keep it simple: no builds for several C libraries or platforms.

## Considered options

1. Keep building every tool from source (ADR-0013).
2. Release binaries pinned by SHA-256 where published; source only where not.
3. A prebuilt CI image containing the tools.

## Decision outcome

Chosen option: **release binaries pinned by SHA-256, and source only where no
binary exists.**

- **Manifest.** `tools/tools.mk` gives, for each binary tool:
  - its repository and version;
  - per platform, the asset name, the SHA-256 the download must match, and the
    binary's path inside the archive.

  Only `linux-amd64` is pinned. Adding a platform means adding one asset and
  hash pair per tool.
- **Fetching.** `tools/fetch.sh` downloads each binary once into
  `.cache/tools/<platform>/<name>-<version>`. It verifies the download against
  the **pinned** hash, never against a checksums file fetched at build time.
  On a mismatch it fails and leaves no binary behind. Make targets depend on
  the binaries they use.
- **Updating.** `make tools-update` moves every tool to its latest release,
  taking hashes from each release's checksums file. `make tools-audit`
  re-derives the current pins from those files and fails on any difference.
- **Source-built tools.** govulncheck and goimports publish no binaries. They
  stay pinned through `tools/<name>/go.mod` and run with `go tool`. They're
  small, about 170 MB each.
- **Platform and prerequisites.** Development and CI run on **glibc Linux
  amd64**, meaning Debian or Ubuntu. The prerequisites are Go, Docker, make,
  and the standard curl, tar and sha256sum. On any other platform, `make shell`
  runs the tools inside the pinned CI image. **The C compiler required by
  ADR-0013 is no longer needed**, because Vale's release binary replaces the
  cgo build.
- **Carried over unchanged from ADR-0013:**
  - anything else that isn't Go runs from a container image pinned by digest;
  - `make` is the only entry point;
  - CI files only run make targets (see ADR-0016 for the staged jobs);
  - all knowledge lives in the repository;
  - UI and API-docs assets are vendored, with no runtime CDN;
  - releases are built by a forge-neutral tool.

### Consequences

- Good: fetching all six binaries takes about 8 seconds and 230 MB on disk,
  instead of 6.5 GB and 3.5 minutes.
- Good: no C compiler, and nothing is compiled for tools.
- Bad: the binaries depend on each project's release process. The pinned
  SHA-256 guarantees we run exactly the bytes we reviewed, but not how they
  were built.
- Bad: each new platform means one more hash per tool. `make tools-update`
  derives them.
- Bad: Vale's Linux binary needs glibc, so Alpine/musl hosts use `make shell`.

### Confirmation

- A tampered hash makes the fetch fail, with no binary left behind (verified
  2026-09-25).
- `make tools-audit` checks every pin against its published checksums.
- The CI image is Debian-based and pinned by digest (ADR-0015).

## More information

- Supersedes [ADR-0013](0013-toolchain-per-tool-modules-and-c-compiler.md).
- Measurements and tests are in ITEM-0014.
