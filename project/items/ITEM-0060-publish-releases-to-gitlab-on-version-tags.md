---
id: ITEM-0060
title: Publish releases to GitLab on version tags
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M05
requirements: [REQ-045]
depends_on: [ITEM-0059]
created: 2026-10-07
closed: 2026-10-07
---

# ITEM-0060: Publish releases to GitLab on version tags

## Goal

`make release` publishes a version tag's release: the image, with its SBOM,
to a registry, and the archives to a GitLab release (ADR-0030), from a
tag-only `release` job that `projctl` allows (ADR-0032).

## Acceptance criteria

- [x] `make release` refuses to run off a `vMAJOR.MINOR.PATCH` tag or without its CHANGELOG section, and publishes the image and, with `GITLAB_TOKEN`, the GitLab release with notes from the CHANGELOG.
- [x] `projctl` allows a GitLab job named `release`, running only `make release`, with one tag rule, and nothing else; tests show each case.
- [x] GitLab's CI runs tag pipelines, with the release stage, and a dry run against a local registry publishes the multi-arch image with its SBOM.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-07: Created from M05's approved design.
- 2026-10-07: Done.
  - **`make release`** needs HEAD at a tag, and `RELEASE_IMAGE`,
    `RELEASE_REGISTRY`, `RELEASE_REGISTRY_USER` and
    `RELEASE_REGISTRY_PASSWORD`. It writes a Docker config for the registry
    into a temporary `DOCKER_CONFIG`, mode 700, which ko reads to push.
    The credentials stay in shell variables, never make's, and the recipe
    isn't echoed. It runs `goreleaser release --clean` with the notes from
    `internal/cmd/releasenotes`, a small command that prints the
    CHANGELOG's `## [VERSION]` section for a `vMAJOR.MINOR.PATCH` tag, and
    refuses any other tag, or a missing or empty section. It has table
    tests.
  - **`.goreleaser.yaml`'s release** is disabled unless `GITLAB_TOKEN` is
    set. Its `gitlab_urls` come from `GITLAB_API_URL` and `GITLAB_URL`,
    with the job token and the generic package registry.
  - **`projctl`** allows GitLab's `release` job as the one exception
    (`gitlabRelease`). It must run only `make release`, and its one rule
    must be exactly `if: $CI_COMMIT_TAG =~ /^v[0-9]+\.[0-9]+\.[0-9]+$/`.
    Then its command doesn't count against `make ci`, and its `rules:`
    isn't a skip key. Tests show:
    - the job accepted;
    - rejections for a job that runs more, a rule for every tag, a second
      rule, `when: manual`, another job running `make release`, and a
      release job on GitHub.

    Its comments cite ADR-0032.
  - **`.gitlab-ci.yml`** runs pipelines for tags, and gains the `release`
    stage and job. The job maps `CI_REGISTRY_IMAGE`, `CI_REGISTRY`,
    `CI_REGISTRY_USER`, `CI_REGISTRY_PASSWORD`, `CI_JOB_TOKEN`,
    `CI_API_V4_URL` and `CI_SERVER_URL` to the Makefile's names.
  - **Checked**, on throwaway local tags and a throwaway branch, all
    deleted, none pushed:
    - `make release` refused HEAD at no tag, a tag with no CHANGELOG
      section, `v0.0.9-rc.1`, and a missing `RELEASE_IMAGE`, each with its
      reason.
    - At `v0.0.1`, with a section, against a temporary `registry:3`, and
      without `GITLAB_TOKEN`, it published one OCI index tagged `0.0.1`,
      `0.0` and `latest`, for linux/amd64 and linux/arm64, with three
      `text/spdx+json` SBOMs, for the index and each image. GoReleaser
      skipped the GitLab release ("release is disabled").
    - The pulled amd64 image ran read-only, and reported `v0.0.1`. The
      arm64 image is linux/arm64, user 65532, labelled `0.0.1`.
  - **Not checked before the first tag:** GitLab's own release step, which
    needs the job token in a tag pipeline. `v0.1.0` is its first run; the
    milestone's verification records it.
