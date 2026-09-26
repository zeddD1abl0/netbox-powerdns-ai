---
id: ITEM-0020
title: Command line, config registry and generated references
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M01
requirements: [REQ-001, REQ-002, REQ-005, REQ-006]
depends_on: [ITEM-0019]
created: 2026-09-27
closed:
---

# ITEM-0020: Command line, config registry and generated references

## Goal

The `nbpdns` binary, with Cobra commands and a Viper-backed config registry
that declares each key once. It generates the configuration and command-line
references.

## Acceptance criteria

- [ ] ADR-0021 records Cobra and Viper, their dependency cost, and what the registry adds on top.
- [ ] Every module added to `go.mod` is listed here with its license, and each license is MIT, BSD, Apache-2.0 or MPL-2.0.
- [ ] `nbpdns version`, `config show` and `completion` work. `make build` produces a static binary in `bin/`.
- [ ] Table-driven tests cover precedence (defaults < file < env < flags), `_FILE` secrets at their plain form's level, both forms set, unknown file keys and `NBPDNS_*` variables, source reporting, and `config.Secret` redaction.
- [ ] `make generate` writes `docs/reference/configuration.md` and the command-line reference. `make generate-check` is part of `make check`, and fails on a stale page.
- [ ] A `build` CI job runs on both forges, and `make project-lint` passes.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-09-27: Created from M01's approved design, before implementation
  started.
