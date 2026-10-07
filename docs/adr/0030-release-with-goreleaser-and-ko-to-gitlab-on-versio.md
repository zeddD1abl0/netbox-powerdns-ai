---
title: "0030: Release with GoReleaser and ko to GitLab on version tags"
status: accepted
date: 2026-10-07
decision-makers: [jordan]
requirements: [REQ-002, REQ-003, REQ-045]
questions: [Q-025]
supersedes:
---

# 0030: Release with GoReleaser and ko to GitLab on version tags

## Context and problem statement

Until M05, nbpdns is built only by `make build`: one static binary, for the
developer's own platform. Running it as a service (M04) needs releases that
people can install and deploy:
- binaries for the platforms it runs on;
- a container image;
- a way to check that a download is what was published;
- somewhere they're published.

Each release must be built the same way every time.

## Decision drivers

- Self-contained, with every tool pinned (ADR-0022), and CI files that only
  run make targets (ADR-0032).
- Linux only, amd64 and arm64 (Q-025's default).
- The same commit builds the same bytes.
- Nothing published by accident: only a deliberate, version-shaped tag
  publishes.
- A small, non-root runtime image (ADR-0031).

## Considered options

1. **Build tool:** GoReleaser; hand-written make targets around `go build`
   and `tar`.
2. **Image tool:** ko, through GoReleaser; a Dockerfile, built with buildx.
3. **Publishing:** GitLab, on version tags; build only, publishing later;
   GitHub, from the mirror.
4. **Supply chain:** an image SBOM now, with signing later; signing with a
   cosign key; keyless signing through Sigstore; neither.

## Decision outcome

The user chose the publishing, the supply chain and the first release on
2026-10-07. The tools follow from the drivers.

**GoReleaser** (MIT), pinned as a release binary by SHA-256 (ADR-0022),
builds the release:
- **Binaries:** `CGO_ENABLED=0`, `-trimpath`, `-ldflags=-s -w`, for
  linux/amd64 and linux/arm64. Every file's time is that of the commit, so
  the same commit builds byte-identical archives. The version comes from Go's
  build info, as it does for `make build`: a build at tag `v0.1.0` reports
  `v0.1.0`.
- **Archives:** `nbpdns_<version>_linux_<arch>.tar.gz`, each holding the
  binary, `LICENSE`, `README.md` and `CHANGELOG.md`, with `checksums.txt`,
  in SHA-256.
- **The image**, built by GoReleaser's ko integration: no Dockerfile, no
  Docker CLI, and no build step in a shell.
  - It's one multi-arch index for amd64 and arm64, on the runtime image of
    ADR-0031.
  - Its tags are `<version>`, `<major>.<minor>` and `latest`.
  - It has the OCI labels: source, version, revision, created, licenses,
    title and description.
  - It runs `nbpdns` as user 65532.
  - ko attaches an SPDX SBOM to it in the registry.

**Publishing, to GitLab, on a `vMAJOR.MINOR.PATCH` tag**, through
`make release`:
- The image goes to the registry that `RELEASE_IMAGE` names, with the
  `RELEASE_REGISTRY*` credentials.
- The archives and checksums go to a GitLab release, stored in the generic
  package registry, with the CI job token. The GitLab release step runs
  only when `GITLAB_TOKEN` is set.
- The release notes are the version's section of the CHANGELOG.
- `make release` refuses to run unless HEAD is at such a tag and the
  CHANGELOG has its section.
- Only the GitLab CI file maps GitLab's variables to these names, so the
  Makefile and `.goreleaser.yaml` name no forge.

**Every other pipeline builds and tests the release**, with
`make release-check`, which publishes nothing. In GoReleaser's snapshot
mode, the image goes to the local Docker daemon.

**Supply chain:** the image's SBOM now, and SHA-256 checksums for the
archives. Signing waits for M17, where key management and the threat model
are designed together.

**Versions** are semantic, 0.x until v1.0 (M19). The `[Unreleased]`
section of the CHANGELOG becomes the release's when it's tagged. The first
release is v0.1.0, which the user tags after M05 is merged.

### Consequences

- Good: a release is one tag. What's published is what every pipeline
  built and tested.
- Good: a download can be checked against `checksums.txt`, and the image's
  contents against its SBOM.
- Good: there's no Dockerfile to maintain, and ko builds images without a
  daemon when it publishes.
- Bad: GoReleaser and ko own the release's layout; changing it means
  changing their configuration, not a script.
- Bad: nothing is signed until M17, so a checksum proves integrity, not
  origin.
- Bad: snapshot builds of the image need a Docker daemon, so the release
  check runs with Docker-in-Docker, as the integration tests do.

### Confirmation

- `make release-check`, in every pipeline, builds the archives for both
  architectures and the image, and tests them: static binaries of the
  right architecture, the version, the checksums, and the image's user,
  labels, read-only run, and missing shell.
- Two builds of the same commit give the same `checksums.txt`.
- The first tag, `v0.1.0`, publishes the multi-arch image and a GitLab
  release with the archives.

## Pros and cons of the options

### Hand-written make targets

- Good: no new tool.
- Bad: cross-compiling, archiving, checksums, multi-arch images and GitLab
  releases are each a script to write and test, which GoReleaser already is.

### A Dockerfile with buildx

- Good: familiar, and any build step is possible.
- Bad: it needs the Docker CLI and buildx pinned, and a daemon to publish.
  A static Go binary needs no build step that ko lacks.

### Signing now

- With a cosign key: one more secret to hold and rotate before the threat
  model (M17) says how.
- Keyless, through Sigstore: no key, but every signature records the GitLab
  instance's identity in the public Rekor log.

## More information

- ADR-0022 (pinned tools), ADR-0031 (the runtime image), ADR-0032 (the CI
  stages and the release job).
- Recorded in M05's design, 2026-10-07. Implemented by ITEM-0058 to
  ITEM-0061.
