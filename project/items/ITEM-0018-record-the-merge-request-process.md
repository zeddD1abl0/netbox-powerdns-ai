---
id: ITEM-0018
title: Record the merge-request process
type: task # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M01
requirements: [REQ-012, REQ-026]
depends_on: []
created: 2026-09-27
closed: 2026-09-29
---

# ITEM-0018: Record the merge-request process

## Goal

Record how milestones merge, now that `main` exists and GitHub is a push
mirror: through a GitLab merge request with a merge commit. Also have the
`close-milestone` skill write the MR title and description.

## Acceptance criteria

- [x] ADR-0018: GitLab is the primary forge. Milestones merge through an MR with the Merge commit method; squash and fast-forward aren't used. GitHub is a push mirror. The fallback is a local `git merge --no-ff`, then pushing `main` to both remotes. It notes that M00 was fast-forwarded.
- [x] The commits section of `CLAUDE.md` says the user merges through a GitLab MR with a merge commit (ADR-0018).
- [x] Step 8 of the `close-milestone` skill produces the MR title and a paste-ready description with these sections: summary, what's included, verification, reviewing, known and deferred, merging. Paths are shown as code, not links.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-09-27: Created from M01's approved design, before implementation
  started.
- 2026-09-29: Done. ADR-0018 records the decision. The user confirmed that
  GitLab's merge method is Merge commit and squash is disallowed, so that part
  of M01's hand-off is complete. `CLAUDE.md` stays at 150 lines: the two
  commit bullets were merged into one. The `close-milestone` skill now writes
  the MR title and description. The M00 description wasn't saved, so the
  sections come from the approved design.
