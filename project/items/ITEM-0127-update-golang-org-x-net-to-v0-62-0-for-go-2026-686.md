---
id: ITEM-0127
title: Update golang.org/x/net to v0.62.0 for GO-2026-6860 and GO-2026-6866
type: debt # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: []
depends_on: []
created: 2026-11-25
closed: 2026-11-25
---

# ITEM-0127: Update golang.org/x/net to v0.62.0 for GO-2026-6860 and GO-2026-6866

## Goal

govulncheck reports GO-2026-6860 and GO-2026-6866 in `golang.org/x/net`
v0.60.0's `html` package, fixed in v0.62.0. Only the API's contract
checker, which the tests use, reaches that package, through
libopenapi-validator; the binary doesn't, and `make vuln` passes. But
the binary's module list names x/net v0.60.0, which scanners that go by
module version flag. The user chose to bump it on 2026-11-25.

## Acceptance criteria

- [x] `go.mod` requires `golang.org/x/net` v0.62.0, and `make vuln` no longer reports GO-2026-6860 or GO-2026-6866.
- [x] `make check` and `make test-integration` pass.
- [x] The CHANGELOG's `Unreleased` says so, under Security.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-11-25: Done. `go get golang.org/x/net@v0.62.0` also raised, as
  x/net requires, x/crypto to v0.58.0, x/sync to v0.24.0, x/sys to v0.49.0
  and x/text to v0.43.0, all under the licenses they had. `make vuln`
  reports neither x/net finding. GO-2026-5932 in x/crypto stays: its fix
  is only in a pseudo-version after v0.58.0, and the code doesn't call it.
