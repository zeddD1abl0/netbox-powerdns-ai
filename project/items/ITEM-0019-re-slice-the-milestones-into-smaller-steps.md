---
id: ITEM-0019
title: Re-slice the milestones into smaller steps
type: task # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M01
requirements: [REQ-022]
depends_on: []
created: 2026-09-27
closed: 2026-09-29
---

# ITEM-0019: Re-slice the milestones into smaller steps

## Goal

Replace the eight large milestones with the eighteen smaller ones in M01's
approved design. Each merge becomes a useful, logical step, and
authentication comes after the read-only features.

## Acceptance criteria

- [x] ADR-0019 records the re-slice, with a table mapping old milestones to new ones, so references in accepted ADRs can still be resolved.
- [x] The stubs M01 to M08 are rewritten, and renamed where the title changed. New stubs exist for M09 to M18.
- [x] `CLAUDE.md` allows exactly one exception to "files never move": a planned milestone stub with no work yet may be rewritten or renamed by a re-plan recorded in an ADR. It stays at 150 lines or fewer.
- [x] Each open question's "Needed by" in `requirements.md` follows the table in M01's approved design.
- [x] The milestone numbers in `docs/*/_index.md` are updated.
- [x] The branch is renamed to `m01-netbox-read-path`.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-09-27: Created from M01's approved design, before implementation
  started.
- 2026-09-29: Done. ADR-0019 has the new list, where each old milestone's
  scope went, and where each old-milestone reference in the accepted ADRs
  now points. M01 to M08 were renamed with `git mv` and rewritten, and M09 to
  M18 are new. M01 keeps its approved design unchanged, under a new header
  and a "Decided after approval" callout. `CLAUDE.md` stays at 150 lines.
  Also done here, as agreed in the review:
  - `project/brief.md` has the 2026-09-27 and 2026-09-29 answers;
  - the outdated M0/M1 text in `README.md`, `docs/_index.md`, `CLAUDE.md` and
    the Makefile is fixed;
  - ITEM-0017 waits for this item, so the board follows the phase order;
  - ITEM-0025 is filed for the missing C compiler prerequisite.

  Q-007 now says "Needed by M01". It moves to Answered with ADR-0020, the
  next commit. The branch is now `m01-netbox-read-path`.
