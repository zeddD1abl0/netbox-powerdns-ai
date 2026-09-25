---
id: ITEM-0005
title: projctl — scaffold, index and lint the project tracking files
type: feature
status: open
milestone: M00
requirements: [REQ-012, REQ-025]
depends_on: [ITEM-0004]
created: 2026-09-25
closed:
---

# ITEM-0005: projctl — scaffold, index and lint the project tracking files

## Goal

Build a small Go tool in `tools/projctl` that makes the tracking model in
ADR-0002 mechanical, so it can't drift.

## Acceptance criteria

- [ ] `projctl new item "<title>"` and `projctl new adr "<title>"` scaffold a file with the next free ID.
- [ ] `projctl index` regenerates:
  - `project/README.md`: milestones, items grouped by status, questions still open, and requirement coverage;
  - the ADR table in `docs/adr/_index.md`.
- [ ] `projctl lint` fails when:
  - an ID is duplicated;
  - front matter is invalid or a status value is unknown;
  - an item names a milestone, REQ, Q or ITEM that doesn't exist;
  - a milestone is `done` but still has open items;
  - an item is `done` without a `closed` date;
  - a generated file is stale;
  - a relative link or anchor doesn't resolve.
- [ ] Table-driven tests cover every lint rule, with fixtures.
- [ ] The `new-item`, `new-adr` and `close-milestone` skills call `projctl` instead of the manual steps.
- [ ] `CLAUDE.md` no longer says the board is maintained by hand.

## Notes
