---
id: ITEM-0011
title: Cold-start test and M0 close
type: task
status: open
milestone: M00
requirements: [REQ-022]
depends_on: [ITEM-0002, ITEM-0003, ITEM-0004, ITEM-0005, ITEM-0006, ITEM-0007, ITEM-0008, ITEM-0009, ITEM-0010]
created: 2026-09-25
closed:
---

# ITEM-0011: Cold-start test and M0 close

## Goal

Prove that the setup can be repeated: a fresh session can orient itself from
the repo alone.

## Acceptance criteria

- [ ] A fresh Claude session is given only "Read CLAUDE.md, then tell me the current status and the next item." It answers correctly after reading 5 or fewer files.
- [ ] Any confusion it shows is fixed in `CLAUDE.md` or the board, and the test re-run.
- [ ] The `close-milestone` skill has run for M00.
- [ ] The branch is handed over for the user to review and merge (ADR-0010).

## Notes
