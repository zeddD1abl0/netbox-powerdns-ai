---
id: ITEM-0060
title: Publish releases to GitLab on version tags
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M05
requirements: [REQ-045]
depends_on: [ITEM-0059]
created: 2026-10-07
closed:
---

# ITEM-0060: Publish releases to GitLab on version tags

## Goal

`make release` publishes a version tag's release: the image, with its SBOM,
to a registry, and the archives to a GitLab release (ADR-0030), from a
tag-only `release` job that `projctl` allows (ADR-0032).

## Acceptance criteria

- [ ] `make release` refuses to run off a `vMAJOR.MINOR.PATCH` tag or without its CHANGELOG section, and publishes the image and, with `GITLAB_TOKEN`, the GitLab release with notes from the CHANGELOG.
- [ ] `projctl` allows a GitLab job named `release`, running only `make release`, with one tag rule, and nothing else; tests show each case.
- [ ] GitLab's CI runs tag pipelines, with the release stage, and a dry run against a local registry publishes the multi-arch image with its SBOM.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-07: Created from M05's approved design.
