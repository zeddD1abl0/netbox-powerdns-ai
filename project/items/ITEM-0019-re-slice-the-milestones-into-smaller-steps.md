---
id: ITEM-0019
title: Re-slice the milestones into smaller steps
type: task # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M01
requirements: [REQ-022]
depends_on: []
created: 2026-09-27
closed:
---

# ITEM-0019: Re-slice the milestones into smaller steps

## Goal

Replace the eight large milestones with the eighteen smaller ones in M01's
approved design. Each merge becomes a useful, logical step, and
authentication comes after the read-only features.

## Acceptance criteria

- [ ] ADR-0019 records the re-slice, with a table mapping old milestones to new ones, so references in accepted ADRs can still be resolved.
- [ ] The stubs M01 to M08 are rewritten, and renamed where the title changed. New stubs exist for M09 to M18.
- [ ] `CLAUDE.md` allows exactly one exception to "files never move": a planned milestone stub with no work yet may be rewritten or renamed by a re-plan recorded in an ADR. It stays at 150 lines or fewer.
- [ ] Each open question's "Needed by" in `requirements.md` follows the table in M01's approved design.
- [ ] The milestone numbers in `docs/*/_index.md` are updated.
- [ ] The branch is renamed to `m01-netbox-read-path`.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-09-27: Created from M01's approved design, before implementation
  started.
