---
id: ITEM-0007
title: Lint configuration for Go, OpenAPI and documentation
type: task
status: done
milestone: M00
requirements: [REQ-019, REQ-020, REQ-021]
depends_on: [ITEM-0004]
created: 2026-09-25
closed: 2026-09-25
---

# ITEM-0007: Lint configuration for Go, OpenAPI and documentation

## Goal

Encode the engineering and documentation standards as linters, so they're
checked rather than remembered.

## Acceptance criteria

- [x] `.golangci.yml` (v2 format) with a curated linter set, and a written reason for each exclusion.
- [x] Vale config with the Google style package **vendored** in `.vale/styles/Google`, plus a project vocabulary (NetBox, PowerDNS, LightningStream, and so on).
- [x] Vale rules for the banned words in the documentation style guide (`.vale/styles/nbpdns/Banned.yml`).
- [x] A Zalando-based vacuum ruleset (`api/ruleset.yaml`). There are no deviations to link to ADR-0012.
- [x] `make lint`, `make docs-lint` and `make api-lint` each run and pass on the current tree.

## Notes

- 2026-09-25: golangci-lint runs the `standard` set plus 17 linters, each with
  a one-line reason. The fixes it found in projctl were exhaustive `default`
  cases, closing errors joined properly, test cleanup errors checked, and
  unused parameters renamed. gosec's file-permission and walk-race checks are
  excluded for projctl only, with the reason written in the config.
- 2026-09-25: Vale:
  - `make docs-lint` fails on **any** finding. Vale itself exits 0 on
    warnings, so the target checks for output instead.
  - Accepted ADRs can't be edited (ADR-0001), so they get spelling and banned
    words only, and `Vale.Terms` is off for them.
  - Docs fixed where the Google rules were right: Oxford commas, "CLI", "will",
    quotation punctuation, and US spelling. The style guide now states US
    spelling, the Oxford comma, and the ADR exception.
  - The style guide lists the banned words as code, so they don't trip their
    own rule.
- 2026-09-25: vacuum:
  - It doesn't support Spectral's `field: "@key"`; the value comes through
    empty. Keys are selected with JSONPath's `~` instead.
  - Response codes are filtered with RFC 9535 `match()`.
  - vacuum's recommended `camel-case-properties` is off, because it
    contradicts Zalando [118].
  - The rules use only core functions, so nothing needed JavaScript.
  - The self-test is in `make api-lint`: `api/testdata/good.yaml` must pass
    (it scores 100/100 and doubles as a model spec), and `bad.yaml` must
    trigger every `zalando-*` rule.
- 2026-09-25: Negative checks:
  - breaking zalando-115 made `make api-lint` fail ("bad.yaml doesn't trigger
    zalando-115-no-version-in-path");
  - adding "simply" to a doc made `make docs-lint` fail.
