---
id: ITEM-0125
title: Update Go to 1.27.4 for GO-2026-6859
type: debt # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: []
depends_on: []
created: 2026-11-25
closed: 2026-11-25
---

# ITEM-0125: Update Go to 1.27.4 for GO-2026-6859

## Goal

On 2026-11-25, working on ITEM-0124, `make vuln` failed on GO-2026-6859
(CVE-2026-97054), published on 2026-11-11: in Go 1.27.3, an HTTP/1 server
whose handler reads a body through `http.MaxBytesReader` can crash, as the
reader sets `Connection: close` without a lock while the handler may write
the response's headers. nbpdns limits a webhook's body that way
(`internal/api/events.go`), and govulncheck reaches `net/http` from many
places. Every module's toolchain and the CI image move to Go 1.27.4, as
ITEM-0118 did for 1.27.3, so that `make check`, and every pipeline, is
green again.

## Acceptance criteria

- [x] Every module's `toolchain` is go1.27.4.
- [x] `CI_IMAGE` and both forges' jobs use `golang:1.27.4`, pinned by its index digest, still Debian 13 with gcc.
- [x] `make check` and `make release-check` pass.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-11-25: Created from ITEM-0124, whose `make check` failed only on
  `make vuln`. `golang:1.27.4`'s index digest is
  `sha256:5f1b98fa02fe396ebce2a0227a0e08309e11792915b9dd2e1f71053d7e7e6666`:
  Debian 13 (trixie), Go 1.27.4, gcc 14.2.0, as the 1.27.3 image was.
  `GOTOOLCHAIN=auto` fetched go1.27.4 from the module proxy, checked
  against the checksum database. nbpdns's one `MaxBytesReader` reads the
  webhook's whole body before the handler answers, so the race isn't known
  to reach it.
- 2026-11-25: Done. Every module's toolchain is go1.27.4, and `CI_IMAGE`,
  GitLab's jobs and GitHub's use `golang:1.27.4` by its index digest. The
  CHANGELOG's unreleased Go line now names 1.27.4 and both fixes, since
  1.27.3 was never released. govulncheck finds nothing that the code calls.
  The root module's reports that the code doesn't call are GO-2026-5932, in
  x/crypto, as before, and two new ones in `golang.org/x/net` v0.60.0,
  GO-2026-6860 and GO-2026-6866, fixed in v0.62.0.
