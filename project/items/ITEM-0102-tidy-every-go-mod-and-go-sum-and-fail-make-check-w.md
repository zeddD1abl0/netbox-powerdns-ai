---
id: ITEM-0102
title: Tidy every go.mod and go.sum, and fail make check when one isn't
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-022]
depends_on: []
created: 2026-10-14
closed: 2026-10-15
---

# ITEM-0102: Tidy every go.mod and go.sum, and fail make check when one isn't

## Goal

`go mod tidy -diff` fails for the main module and for two of the tool
modules, and nothing in `make check` notices. In the main `go.mod`,
`github.com/pressly/goose/v3` and `modernc.org/sqlite` are marked
`// indirect`, though `internal/store` has imported both since 35bea14
(ITEM-0086). Its `go.sum` lacks hashes that the module graph needs and
keeps two from older versions. Tidy every module, and make `make check`,
and so CI, fail when one isn't tidy, so the next drift is caught when it
happens.

## Acceptance criteria

- [x] `go mod tidy -diff` reports nothing for the main module or for any
  `tools/*/go.mod`.
- [x] A make target fails, naming the module, when any of them isn't tidy,
  and `make check` and a CI job run it.
- [x] `make check` passes, and each tool still runs from its module.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-14: Found when the `gopls` language server reported, in
  `go.mod`, that goose and modernc's sqlite "should be direct". With Go
  1.27.2, `go mod tidy -diff` prints what tidy would change, without
  changing anything, and exits 1 when there's a change. For the main
  module, it moves goose and sqlite to the direct `require` block. In
  `go.sum`, it adds hashes for `github.com/google/pprof`,
  `github.com/hashicorp/golang-lru/v2`, `golang.org/x/mod`,
  `golang.org/x/tools` and ten `modernc.org` modules, among them `cc/v4`,
  `ccgo/v4` and `gc/v3`; it drops `klauspost/compress` v1.19.1 and
  `prometheus/procfs` v0.21.1, left from before their updates; and it adds
  the `/go.mod` hash for `klauspost/compress` v1.19.2. Of the tool
  modules, `tools/goimports` (2 lines) and `tools/govulncheck` (10 lines)
  aren't tidy; `hooktest`, `oapi-codegen` and `projctl` are.
- 2026-10-15: Done. `make tidy` tidied the main module (goose and sqlite
  moved to the direct `require` block; `go.sum` gained 29 lines and lost
  the 4 stale ones), `tools/goimports` and `tools/govulncheck` (`go.sum`
  only). No version changed. `make tidy-check` runs `go mod tidy -diff` in
  `.` and every `tools/*/go.mod`, found by wildcard, so a new tool module,
  such as ITEM-0103's, is checked without a change. It names each untidy
  module and prints its diff. The command exits 1 both for a change and
  for a failure, so an empty diff is reported as a failure, not as an
  untidy module. `make check` runs it, and CI's `go-lint` job, on both
  forges, runs `make vet lint generate-check tidy-check`.
  The network: in the pinned CI image, with `GOTOOLCHAIN=local`, the module
  cache is empty, and `tidy -diff` needs the network for every module that
  has dependencies. It still does after loading what `vet` loads: the main
  module wants test-only modules, such as go-jsoncommentstrip. Every job
  has the network, and `go-lint` already holds most of what tidy reads
  (`generate-check` builds oapi-codegen), so tidy adds about 190 MB and
  11 s there, where a job of its own would fetch about 860 MB cold. The
  user chose `go-lint`. Checked: before tidying, the target failed with
  `not tidy: . tools/goimports tools/govulncheck`. With `GOPROXY=off` and
  an empty module cache, it failed with `.: go mod tidy -diff failed`.
  goimports ran from its module as the edit hook runs it, and govulncheck,
  oapi-codegen, projctl and hooktest ran through `make check`. The
  CHANGELOG gained a line under Added, as 0.7.0's make targets did, and
  `CLAUDE.md`'s description of `make check` names tidiness.
