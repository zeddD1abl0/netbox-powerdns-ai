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
- [x] Each milestone file has a GitLab milestone, titled `Mnn: Title`, and M00 to M07 are closed.
- [ ] The branch is merged, and the user tags its merge commit `v0.7.0`; the `[0.7.0]` section's date is the merge's.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-09: The user chose the Planner token, `v0.NN.0` releases (after
  `v1.0.0-mNN`, which they tagged and then deleted), and planning branches,
  and accepted ADR-0037 and its CLAUDE.md changes. The `[0.7.0]` section
  is dated 2026-10-09; if the branch merges later, its date moves to the
  merge's.
- 2026-10-09: Created GitLab milestones %2 to %21, one for each milestone
  file, M00 to M19, through the API with the Planner token. Each is titled
  `Mnn: Title`, and its description is the file's goal and path. M00 to
  M07 are closed, with their start and close dates; GitLab refuses a due
  date that isn't after the start date, so those done in a day (M02, M04,
  M06) have only a due date. M08 to M19 are open. The user's test
  milestone and issue, `claude-test`, are left as they are.
- 2026-10-09: The branch is ready to close. It planned M08 in full and M09
  to M13 as roadmap entries, and re-planned M11 to M20 (ADR-0043). Checks:
  `make check` passed; `make test-integration` passed (internal/cli 47.5 s,
  internal/netbox 9.1 s, internal/lab 1.1 s, internal/powerdns 1.2 s); the
  live docs name the new milestone numbers, leaving them only in ADR-0043's
  mapping and in history. The user chose to close it without `/code-review
  high`, since its code changes are one string and comments. This item's
  last criterion waits for the merge and the `v0.7.0` tag; the `[0.7.0]`
  section is dated 2026-10-09, the day it's meant to merge.
