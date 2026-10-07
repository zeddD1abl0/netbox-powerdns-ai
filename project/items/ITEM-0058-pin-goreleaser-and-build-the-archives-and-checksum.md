---
id: ITEM-0058
title: Pin GoReleaser and build the archives and checksums
type: feature # feature | bug | debt | task
status: in-progress # open | in-progress | blocked | done | wontfix
milestone: M05
requirements: [REQ-002, REQ-045]
depends_on: []
created: 2026-10-07
closed:
---

# ITEM-0058: Pin GoReleaser and build the archives and checksums

## Goal

Pin GoReleaser (ADR-0022, ADR-0030), and have it build nbpdns's static
linux amd64 and arm64 binaries into archives with `checksums.txt`, the
same bytes from the same commit.

## Acceptance criteria

- [ ] GoReleaser is pinned by SHA-256 in `tools/tools.mk`, and `.goreleaser.yaml` builds `CGO_ENABLED=0`, `-trimpath`, `-s -w` binaries for linux/amd64 and linux/arm64.
- [ ] The archives hold the binary, `LICENSE`, `README.md` and `CHANGELOG.md`, with `checksums.txt`, and two builds of one commit are byte-identical.
- [ ] A snapshot build's binary reports its pseudo-version, and a tagged build its tag.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-07: Created from M05's approved design.
