---
id: ITEM-0050
title: Split packaging into its own milestone
type: task # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M04
requirements: [REQ-002, REQ-003]
depends_on: []
created: 2026-10-07
closed:
---

# ITEM-0050: Split packaging into its own milestone

## Goal

Split packaging out of M04 into its own milestone, M05 "Packaging", and
shift the planned stubs after it up one number (ADR-0028). Live references
and the text nbpdns prints use the new numbers; records keep theirs.

## Acceptance criteria

- [ ] ADR-0028 is accepted, and the stubs M05 to M18 are M06 to M19, with M05 "Packaging" new: their files, IDs, titles and branch names.
- [ ] Open items, the open questions' "needed by" column, the docs, the CHANGELOG, `CLAUDE.md` and code text use the new numbers; `enforce (from M12)` is `enforce (from M13)`.
- [ ] `make project-lint` and `make check` pass.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-07: Created from M04's approved design.
