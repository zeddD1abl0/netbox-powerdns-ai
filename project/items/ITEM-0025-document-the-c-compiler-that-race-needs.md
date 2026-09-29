---
id: ITEM-0025
title: Document the C compiler that -race needs
type: bug # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M01
requirements: [REQ-022, REQ-026]
depends_on: [ITEM-0020]
created: 2026-09-29
closed:
---

# ITEM-0025: Document the C compiler that -race needs

## Goal

The documented prerequisites are wrong. `CLAUDE.md`, `README.md` and ADR-0014
list Go, Docker, make, curl, tar and sha256sum, and ADR-0014 says a C compiler
is no longer needed. But every test runs with `-race`, and the race detector
needs cgo. On a host with only the listed prerequisites, Go disables cgo and
`make test` fails with `go: -race requires cgo`. CI never shows this, because
the golang image ships gcc.

## Acceptance criteria

- [ ] A new ADR supersedes ADR-0014. It restates it with a C compiler among the prerequisites (`gcc` and `libc6-dev` on Debian or Ubuntu), because the race detector needs cgo. It's numbered after ADR-0021.
- [ ] `CLAUDE.md` and `README.md` list the C compiler and link the new ADR, and so does REQ-026's source in `project/requirements.md`.
- [ ] Without cgo, `make test` and `make test-integration` stop with a message that names the missing C compiler.
- [ ] Checked both ways: on a `PATH` without gcc the message appears, and with gcc `make check` passes.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-09-29: Found in the review before M01 implementation, and reproduced:
  on a `PATH` with no gcc, `go env CGO_ENABLED` is `0` and
  `go test -race ./...` fails with `go: -race requires cgo; enable cgo by
  setting CGO_ENABLED=1`. The user accepted the fix proposed in the review.
  It waits for ITEM-0020 only so that its ADR takes the number after ADR-0021
  (Cobra and Viper), keeping the numbers in M01's approved design.
