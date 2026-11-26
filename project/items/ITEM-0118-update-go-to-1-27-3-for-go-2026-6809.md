---
id: ITEM-0118
title: Update Go to 1.27.3 for GO-2026-6809
type: debt # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: []
depends_on: []
created: 2026-10-22
closed: 2026-10-22
---

# ITEM-0118: Update Go to 1.27.3 for GO-2026-6809

## Goal

On 2026-10-22, working on ITEM-0111, `make vuln` failed on GO-2026-6809,
published on 2026-10-21, after ITEM-0110's commit: Go 1.27.2's
`crypto/x509` lacks the denylist of compromised intermediate certificates
that 1.27.3 adds to `Certificate.Verify`, which every TLS client in nbpdns
reaches. Every module's toolchain and the CI image move to Go 1.27.3, as
ITEM-0083 did for 1.27.2, so that `make check`, and every pipeline, is
green again.

## Acceptance criteria

- [x] Every module's `toolchain` is go1.27.3.
- [x] `CI_IMAGE` and both forges' jobs use `golang:1.27.3`, pinned by its index digest, still Debian 13 with gcc.
- [x] `make check` and `make release-check` pass.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-22: Created from ITEM-0111, whose `make check` failed only on
  `make vuln`. `golang:1.27.3`'s index digest is
  `sha256:f20d83a83e18dbd2a4714b8ffd4b30b287b6243962dd75608e86abcc63ba4efc`:
  Debian 13 (trixie), Go 1.27.3, gcc 14.2.0, as the 1.27.2 image was.
  `GOTOOLCHAIN=auto` fetched go1.27.3 from the module proxy, checked
  against the checksum database.
- 2026-10-22: Done. Every module's toolchain is go1.27.3, and `CI_IMAGE`,
  GitLab's jobs and GitHub's use `golang:1.27.3` by its index digest.
  `make check` and `make release-check` pass on it, and govulncheck finds
  nothing that the code calls; the root module's only report is still
  GO-2026-5932, in x/crypto, which the code doesn't call.
