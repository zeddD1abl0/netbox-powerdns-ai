---
id: ITEM-0059
title: The container image with ko and the release tests
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M05
requirements: [REQ-003, REQ-045]
depends_on: [ITEM-0058]
created: 2026-10-07
closed: 2026-10-07
---

# ITEM-0059: The container image with ko and the release tests

## Goal

Build the multi-arch image with GoReleaser's ko integration, on the pinned
distroless static base (ADR-0031), with its labels and SBOM; test the
archives and the image; and run `make release-check` in both forges' CI.

## Acceptance criteria

- [x] `make release-check` builds the archives and the image, which it loads into the Docker daemon, and runs the release tests; `make ci` runs it.
- [x] The release tests check static binaries of the right architecture, the checksums, the version, and the image: user 65532, the entrypoint, the labels, a read-only run, and no shell.
- [x] Both forges run `release-check` with Docker-in-Docker, and `make project-lint` passes.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-07: Created from M05's approved design.
- 2026-10-07: Done.
  - **`kos`** in `.goreleaser.yaml` builds the image from the `nbpdns`
    build's settings, for linux/amd64 and linux/arm64. Its base is
    `gcr.io/distroless/static-debian13:nonroot`, pinned by its index's
    digest (`sha256:e2e927ec…`), which the registry shows covering both.
    Details:
    - user `65532:65532`;
    - tags `<version>`, `<major>.<minor>` and `latest`, under
      `RELEASE_IMAGE`;
    - `bare`, an SPDX SBOM, and the commit's time as the creation time;
    - the OCI labels: title, description, source, licenses, version,
      revision and created.

    The source label is the module's path, since GoReleaser's `.GitURL` is
    the clone URL, which in GitLab CI carries the job token. The amd64
    image is 21.8 MB.
  - **In snapshot mode**, ko loads the host's platform's image into the
    daemon at `DOCKER_HOST` as `goreleaser.ko.local:<version>`. The daemon
    takes one platform, so the arm64 image is only checked by ITEM-0060's
    dry run to a registry.
  - **`internal/release`'s image tests** use the Docker Engine API directly,
    over a TCP address or a Unix socket, with the standard library, and
    check:
    - the config: user 65532, the `nbpdns` entrypoint, and every label,
      with the build's version and revision;
    - `version -o json`, run with a read-only root file system and no
      network, giving the build's commit;
    - `serve` with no groups, exiting 1 with its message;
    - `/bin/sh`, which can't start, since there's no shell.

    They skip for a tag's build, whose image is pushed rather than loaded.
  - **CI:** `make ci` runs `release-check`. Both forges have a
    `release-check` job in the build stage, with a Docker-in-Docker
    service, without the lab: GitLab through `DOCKER_HOST`, GitHub through
    `LAB_DOCKER_HOST`, which the Makefile's global mapping already covers.
    The CI files and the Makefile now cite ADR-0032 and ADR-0031.
  - **Checked:** `make release-check` passes on the local daemon, and
    against a temporary Docker-in-Docker container over TCP, with
    `LAB_DOCKER_HOST` set, as in CI.
