---
id: ITEM-0059
title: The container image with ko and the release tests
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M05
requirements: [REQ-003, REQ-045]
depends_on: [ITEM-0058]
created: 2026-10-07
closed:
---

# ITEM-0059: The container image with ko and the release tests

## Goal

Build the multi-arch image with GoReleaser's ko integration, on the pinned
distroless static base (ADR-0031), with its labels and SBOM; test the
archives and the image; and run `make release-check` in both forges' CI.

## Acceptance criteria

- [ ] `make release-check` builds the archives and the image, which it loads into the Docker daemon, and runs the release tests; `make ci` runs it.
- [ ] The release tests check static binaries of the right architecture, the checksums, the version, and the image: user 65532, the entrypoint, the labels, a read-only run, and no shell.
- [ ] Both forges run `release-check` with Docker-in-Docker, and `make project-lint` passes.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-07: Created from M05's approved design.
