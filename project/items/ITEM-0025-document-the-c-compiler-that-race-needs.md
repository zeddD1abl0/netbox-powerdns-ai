---
id: ITEM-0025
title: Document the C compiler that -race needs
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M01
requirements: [REQ-022, REQ-026]
depends_on: [ITEM-0020]
created: 2026-09-29
closed: 2026-09-29
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

- [x] A new ADR supersedes ADR-0014. It restates it with a C compiler among the prerequisites (`gcc` and `libc6-dev` on Debian or Ubuntu), because the race detector needs cgo. It's numbered after ADR-0021.
- [x] `CLAUDE.md` and `README.md` list the C compiler and link the new ADR, and so does REQ-026's source in `project/requirements.md`.
- [x] Without cgo, `make test` and `make test-integration` stop with a message that names the missing C compiler.
- [x] Checked both ways: on a `PATH` without gcc the message appears, and with gcc `make check` passes.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-09-29: Found in the review before M01 implementation, and reproduced:
  on a `PATH` with no gcc, `go env CGO_ENABLED` is `0` and
  `go test -race ./...` fails with `go: -race requires cgo; enable cgo by
  setting CGO_ENABLED=1`. The user accepted the fix proposed in the review.
  It waits for ITEM-0020 only so that its ADR takes the number after ADR-0021
  (Cobra and Viper), keeping the numbers in M01's approved design.
- 2026-09-29: Done.
  - ADR-0022 restates ADR-0014 with a C compiler among the prerequisites,
    and with the manifest extensions from ITEM-0017 (`_TAG`, and assets that
    are the binary itself). ADR-0014 is `superseded by ADR-0022`.
  - `CLAUDE.md`, `README.md` and REQ-026 link ADR-0022, and so do the live
    code comments that named ADR-0014. The CHANGELOG, items and M00 keep
    their historical references, as with ADR-0017.
  - `make test` and `make test-integration` first check `go env CGO_ENABLED`.
  - **Checked both ways:**
    - on a `PATH` with Go but no gcc, `make test` stops with "The race
      detector needs cgo, and so a C compiler, which this host lacks", and
      how to install one;
    - with gcc, `make check` passes.
  - Editing the comment in `tools/tools.mk` makes every host fetch the tools
    again, because each binary depends on that file. That's by design, and
    costs a few seconds.
