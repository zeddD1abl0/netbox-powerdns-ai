---
id: ITEM-0010
title: Finalise the milestone list and write the milestone stubs
type: task
status: done
milestone: M00
requirements: [REQ-022]
depends_on: [ITEM-0002]
created: 2026-09-25
closed: 2026-09-25
---

# ITEM-0010: Finalise the milestone list and write the milestone stubs

## Goal

Turn the provisional milestone list in M00's approved design into real
milestone files, starting with M01. Adjust it for the discovery answers.

## Acceptance criteria

- [x] Milestone list M1 to M8 confirmed or revised with the user.
- [x] `project/milestones/M01-*.md` to `M08-*.md` exist as stubs: front matter (`status: planned`), goal and provisional scope. Design, non-goals and acceptance criteria come in each milestone's own plan-mode session.
- [x] The board lists every milestone from its file.

## Notes

- 2026-09-25: The user confirmed the list by approving the plan that contained
  it. Revisions from the M00 provisional list, following the discovery
  answers:
  - M01 adds leader election and the container lab;
  - M04 adds catalog zone membership, drift policies and brownfield import.

  The acceptance criteria changed from "a file for M01 only" to stubs for
  every milestone, so `projctl` has one source for the board.
