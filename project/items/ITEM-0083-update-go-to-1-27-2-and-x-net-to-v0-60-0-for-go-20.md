---
id: ITEM-0083
title: Update Go to 1.27.2 and x/net to v0.60.0 for GO-2026-6617
type: debt # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M07
requirements: []
depends_on: []
created: 2026-10-09
closed: 2026-10-09
---

# ITEM-0083: Update Go to 1.27.2 and x/net to v0.60.0 for GO-2026-6617

## Goal

On 2026-10-09, closing M07, `make vuln` failed on GO-2026-6617, published
since the branch's pipelines ran: a race in the HTTP/2 server's HPACK
encoder, in `golang.org/x/net` v0.59.0 (fixed in v0.60.0) and in Go
1.27.1's standard library (fixed in 1.27.2). govulncheck finds the HTTP/2
code reachable through net/http's client and server. Every module's
toolchain, the CI image and `x/net` move to the fixed releases, so that
`make check`, and every pipeline, is green again.

## Acceptance criteria

- [x] Every module's `toolchain` is go1.27.2, and the root module requires `golang.org/x/net` v0.60.0.
- [x] `CI_IMAGE` and both forges' jobs use `golang:1.27.2`, pinned by its index digest, still Debian 13 with gcc.
- [x] `make check`, `make test-integration` and `make release-check` pass.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-09: `go get golang.org/x/net@v0.60.0` changed nothing else in
  `go.mod`. With `GOTOOLCHAIN=auto`, `go` fetched go1.27.2 from the module
  proxy, checked against the checksum database. `golang:1.27.2`'s index
  digest is
  `sha256:5bc7f572bbaa98885a3a1fd9c0aa76b59e3e14e8628bfc316bbfd0c701e4818c`:
  Debian 13 (trixie), Go 1.27.2, gcc 14.2.0, as the 1.27.1 image was.
  `nbpdns serve` has no TLS listener and sets no `Protocols`, so it speaks
  only HTTP/1.1 and wasn't exposed; the CHANGELOG says so.
