---
id: ITEM-0050
title: Split packaging into its own milestone
type: task # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M04
requirements: [REQ-002, REQ-003]
depends_on: []
created: 2026-10-07
closed: 2026-10-07
---

# ITEM-0050: Split packaging into its own milestone

## Goal

Split packaging out of M04 into its own milestone, M05 "Packaging", and
shift the planned stubs after it up one number (ADR-0028). Live references
and the text nbpdns prints use the new numbers; records keep theirs.

## Acceptance criteria

- [x] ADR-0028 is accepted, and the stubs M05 to M18 are M06 to M19, with M05 "Packaging" new: their files, IDs, titles and branch names.
- [x] Open items, the open questions' "needed by" column, the docs, the CHANGELOG, `CLAUDE.md` and code text use the new numbers; `enforce (from M12)` is `enforce (from M13)`.
- [x] `make project-lint` and `make check` pass.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-07: Created from M04's approved design.
- 2026-10-07: Done. The stubs M05 to M18 are renamed, highest first, to M06
  to M19, with their IDs, headings, branch names and cross-references, and
  `M05-packaging.md` is new. One script shifted every live reference in a
  single pass, so none moved twice: the docs (not the ADRs), the CHANGELOG,
  `CLAUDE.md`, the Makefile's API-lint message, the lab's compose file, the
  requirements apart from Q-038's row (already new), and the code text,
  where `enforce (from M12)` is now `enforce (from M13)`, with its tests
  and the regenerated configuration reference. Q-025's "needed by" is
  M05 (binary, image) and M17. ITEM-0040 moved to M13, with a note. Left
  alone: done milestones, the brief, accepted ADRs, closed items, a
  `projctl` test fixture that names "M09" as a milestone that doesn't
  exist, and the vendored theme's SVG paths.
