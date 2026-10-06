---
id: ITEM-0045
title: Drift fixture and integration tests
type: task # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M03
requirements: [REQ-036, REQ-043]
depends_on: [ITEM-0044]
created: 2026-10-06
closed:
---

# ITEM-0045: Drift fixture and integration tests

## Goal

A drift fixture in the lab, with one of each case from ADR-0027 across the
lab's NetBox and lab-a's primary, and integration tests of `nbpdns drift` on
it. The lab gains no containers.

## Acceptance criteria

- [ ] The fixture creates every case of the approved design in NetBox and on lab-a, and removes it afterwards.
- [ ] Integration tests check each case, as a table and as JSON, with `--zone`, and with a group whose primary can't be read, and their exit codes.
- [ ] `make test-integration` passes, and the lab's containers are unchanged.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Created from M03's approved design.
