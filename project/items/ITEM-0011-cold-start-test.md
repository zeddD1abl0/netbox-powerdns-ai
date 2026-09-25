---
id: ITEM-0011
title: Cold-start test and M0 close
type: task
status: in-progress
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

- [x] A fresh Claude session is given only "Read CLAUDE.md, then tell me the current status and the next item." It answers correctly after reading 5 or fewer files.
- [x] Any confusion it shows is fixed in `CLAUDE.md` or the board, and the test re-run.
- [ ] The `close-milestone` skill has run for M00.
- [ ] The branch is handed over for the user to review and merge (ADR-0010).

## Notes

- 2026-09-25: Cold-start test, run 1. A fresh read-only Explore subagent was
  given only the prompt, plus a request to list the files it read.
  - It read 4 files in `CLAUDE.md`'s order: `CLAUDE.md`, `project/README.md`,
    the M00 file, then ITEM-0011.
  - It answered correctly: M00 in progress, 11 of 12 items closed, ITEM-0011
    next.
  - It flagged that the M00 "Approved design" snapshot still states rules
    that have since been superseded: the user commits, `/api/v1`, and "Go,
    Docker and make" only.
  - Fixes:
    - an IMPORTANT callout above the snapshot, pointing to ADR-0010,
      ADR-0012, ADR-0013 and ADR-0011;
    - `CLAUDE.md` now says a milestone's approved design is a snapshot, and
      ADRs and `CLAUDE.md` win where it disagrees.
- 2026-09-25: Cold-start test, run 2, with the same prompt plus "say what's
  contradictory or confusing".
  - It read the same 4 files and answered correctly.
  - Of the seven points it raised, two were fixed:
    - the close-milestone skill now says forge pipeline results come from the
      user after they push;
    - the callout also notes that `git push` still asks, and that CHANGELOG
      lines are for user-facing changes only.
  - The rest were cosmetic or already handled: the M0/M00 naming, the
    one-off M00 branch origin, "about 5" vs "5 or fewer", and the question
    count, which is correct because Q-056 exists.
