---
id: ITEM-0024
title: nbpdns netbox commands and docs
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M01
requirements: [REQ-018, REQ-020, REQ-021]
depends_on: [ITEM-0023]
created: 2026-09-27
closed:
---

# ITEM-0024: nbpdns netbox commands and docs

## Goal

`nbpdns netbox check`, `zones` and `records`, with the M01 docs and
CHANGELOG, so the milestone is usable end to end.

## Acceptance criteria

- [ ] The commands work as in M01's approved design, including `--output table|json`, with a non-zero exit and a clear message on failure.
- [ ] The docs exist:
  - tutorial: "Read your NetBox DNS data with nbpdns";
  - how-to: "Give nbpdns read-only access to NetBox", "Configure nbpdns" and "Run the development lab";
  - reference: "Supported versions";
  - explanation: "How nbpdns reads NetBox" and "Configuration sources and precedence".
- [ ] `CHANGELOG.md` has lines under Unreleased.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-09-27: Created from M01's approved design, before implementation
  started.
