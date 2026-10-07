---
id: ITEM-0061
title: Release docs and the CHANGELOG as 0.1.0
type: task # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M05
requirements: [REQ-045]
depends_on: [ITEM-0060]
created: 2026-10-07
closed: 2026-10-07
---

# ITEM-0061: Release docs and the CHANGELOG as 0.1.0

## Goal

The M05 docs: the how-tos "Install nbpdns" and "Run nbpdns in a
container", the reference "Release artifacts", the explanation "How nbpdns
is built and released", the contributing page "Make a release", the
README's Install section, and the CHANGELOG as 0.1.0.

## Acceptance criteria

- [x] The pages exist, each checked against a snapshot build.
- [x] The CHANGELOG's Unreleased section is 0.1.0, ready for the user's `v0.1.0` tag.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-07: Created from M05's approved design.
- 2026-10-07: Done.
  - **Pages:**
    - how-tos "Install nbpdns" and "Run nbpdns in a container", with Docker
      and Kubernetes;
    - the reference "Release artifacts";
    - the explanation "How nbpdns is built and released";
    - the contributing page "Make a release".

    The README gains an Install section. It and `docs/_index.md` no longer
    say there's no release. Each section's index links its new page.
  - **Checked against a snapshot build** (`make release-check`, at
    `db441f1`):
    - The install steps: `sha256sum --ignore-missing --check` passed on the
      amd64 archive, and the unpacked binary reported its version.
    - The Docker recipe, against the lab: the snapshot image ran as written,
      with secrets owned by 65532 at mode 0400 in `/run/secrets`,
      `--read-only`, `--cap-drop ALL` and `no-new-privileges`. It became
      ready after its first refresh, and `docker stop` gave exit 0.
    - The image's config, as the reference lists it: entrypoint
      `/ko-app/nbpdns`, working directory `/home/nonroot`, the environment,
      and no exposed ports.
    - Annotated and lightweight tags both stamp the version (a scratch
      clone, tags deleted).
    - The SBOMs, from a throwaway `v0.0.1` published from a scratch clone
      to a temporary `registry:3`, all deleted:
      - each is SPDX 2.3, under the tag `sha256-<hex>.sbom`;
      - each platform's lists 44 Go modules and the base image;
      - the index's lists the base and both images.

      `docker buildx imagetools inspect` listed linux/amd64 and
      linux/arm64/v8, as "Make a release" says.
  - **Not run:** the Kubernetes example. It wasn't applied to a cluster, so
    as not to touch whatever `kubectl` points at.
  - **CHANGELOG:**
    - a docs line;
    - `## [Unreleased]` became `## [0.1.0] - 2026-10-07`, with an empty
      `## [Unreleased]` above it.

    `releasenotes -tag v0.1.0` extracts its 141 lines. If the tag is made
    on a later day, the heading's date can change: only `## [0.1.0]` is
    matched.
