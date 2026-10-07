---
id: ITEM-0058
title: Pin GoReleaser and build the archives and checksums
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M05
requirements: [REQ-002, REQ-045]
depends_on: []
created: 2026-10-07
closed: 2026-10-07
---

# ITEM-0058: Pin GoReleaser and build the archives and checksums

## Goal

Pin GoReleaser (ADR-0022, ADR-0030), and have it build nbpdns's static
linux amd64 and arm64 binaries into archives with `checksums.txt`, the
same bytes from the same commit.

## Acceptance criteria

- [x] GoReleaser is pinned by SHA-256 in `tools/tools.mk`, and `.goreleaser.yaml` builds `CGO_ENABLED=0`, `-trimpath`, `-s -w` binaries for linux/amd64 and linux/arm64.
- [x] The archives hold the binary, `LICENSE`, `README.md` and `CHANGELOG.md`, with `checksums.txt`, and two builds of one commit are byte-identical.
- [x] A snapshot build's binary reports its pseudo-version, and a tagged build its tag.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-07: Created from M05's approved design.
- 2026-10-07: Done.
  - **GoReleaser v2.18.2** (MIT) is pinned in `tools/tools.mk`: the
    `goreleaser_Linux_x86_64.tar.gz` asset, by the SHA-256 in its
    `checksums.txt` (`0a96edc9…`), checked against the download, with the
    binary at the archive's top.
  - **`.goreleaser.yaml`** builds `./cmd/nbpdns` for linux/amd64 and
    linux/arm64 with `CGO_ENABLED=0`, `-trimpath` and `-s -w`, and
    `mod_timestamp` at the commit's time. The archives hold `nbpdns`, owned
    by root, mode 755, and `LICENSE`, `README.md` and `CHANGELOG.md`, mode
    644, all at the commit's time. They come with `checksums.txt`, in
    SHA-256. GoReleaser's own changelog is off; the release notes will come
    from the CHANGELOG (ITEM-0060).
  - **`make release-check`** builds a snapshot into `dist/` and runs
    `internal/release`'s tests, build tag `release`, which `make vet` and
    golangci-lint now cover too. The Makefile gains `comma`, since
    `$(call …)` splits on commas. The tests read GoReleaser's
    `artifacts.json` and `metadata.json`, and check:
    - archives for exactly linux/amd64 and linux/arm64, named by the
      version;
    - their files, modes, owner and times;
    - static ELF binaries for each architecture, with no interpreter or
      dynamic section;
    - `checksums.txt` against every archive;
    - the host's binary's `version`: a pseudo-version of the commit for a
      snapshot, or the tag, unmodified, for a tag's build.
  - **Checked:**
    - Two snapshot builds of one commit, with Go's build cache cleared
      between them, gave identical `checksums.txt`.
    - A build at a throwaway local tag, `v0.0.1-test.1` (never pushed, and
      deleted after), passed the tests, and its binary reported
      `v0.0.1-test.1`.
    - The stripped binary is 19.4 MB, from 28.2 MB.
