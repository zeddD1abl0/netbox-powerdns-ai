---
id: ITEM-0007
title: Lint configuration for Go, OpenAPI and documentation
type: task
status: blocked
milestone: M00
requirements: [REQ-019, REQ-020, REQ-021]
depends_on: [ITEM-0004]
created: 2026-09-25
closed:
---

# ITEM-0007: Lint configuration for Go, OpenAPI and documentation

## Goal

Encode the engineering and documentation standards as linters, so they're
checked rather than remembered.

## Acceptance criteria

- [ ] `.golangci.yml` (v2 format) with a curated linter set and a written reason for each disabled check.
- [ ] Vale config with the Google style package, plus a project vocabulary (NetBox, PowerDNS, LightningStream, and so on).
- [ ] Vale rules for the banned words in the documentation style guide.
- [ ] A vacuum ruleset based on the Zalando guidelines, with any deviations linked to the API-standard ADR (ITEM-0009).
- [ ] All three run in `make lint` and pass on the current tree.

## Notes
