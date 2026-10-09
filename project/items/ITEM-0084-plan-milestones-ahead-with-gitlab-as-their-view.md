---
id: ITEM-0084
title: Plan milestones ahead, with GitLab as their view
type: task # feature | bug | debt | task
status: in-progress # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-025, REQ-026]
depends_on: []
created: 2026-10-09
closed:
---

# ITEM-0084: Plan milestones ahead, with GitLab as their view

## Goal

ADR-0037, which the user accepted on 2026-10-09: plan milestones ahead on
a planning branch, with GitLab's milestones and issues as a view of
`project/` and an intake for items, through a Planner token; and release
each milestone as `v0.NN.0`. This item makes the process real, on
`plan-m08-m12`: CLAUDE.md, the `close-milestone` skill and "Make a
release", the CHANGELOG's `[0.7.0]` section, and the GitLab milestones.
Each milestone that the branch designs has an item of its own.

## Acceptance criteria

- [x] ADR-0037 is accepted, and CLAUDE.md, the `close-milestone` skill and "Make a release" follow it.
- [x] The CHANGELOG's M06 and M07 entries are under `[0.7.0]`, which `releasenotes -tag v0.7.0` prints.
- [ ] Each milestone file has a GitLab milestone, titled `Mnn: Title`, and M01 to M07 are closed.
- [ ] The branch is merged, and the user tags its merge commit `v0.7.0`; the `[0.7.0]` section's date is the merge's.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-09: The user chose the Planner token, `v0.NN.0` releases (after
  `v1.0.0-mNN`, which they tagged and then deleted), and planning branches,
  and accepted ADR-0037 and its CLAUDE.md changes. The `[0.7.0]` section
  is dated 2026-10-09; if the branch merges later, its date moves to the
  merge's.
