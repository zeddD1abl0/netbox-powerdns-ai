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
- 2026-09-25: **Session ended here. State for the next session:**
  - `m00-foundation` is at `1a68497` on both `origin` (GitLab) and `github`,
    and the tree is clean. `make ci` is green locally, and each CI job passed
    cold in the pinned image (ITEM-0015 notes).
  - **Waiting on the user.** They'll report:
    - the GitLab and GitHub pipeline results for `1a68497`, the first run of
      the staged pipelines;
    - whether GitLab's file view renders the NOTE alert in `docs/_index.md`
      and the Mermaid diagram in ADR-0007.

    If a pipeline fails, fix it on this branch: under a new M00 item, or
    here if it's small. Then ask the user to push again. Claude never pushes.
  - **Before closing M00, re-run `/code-review high`.** The first review
    (ITEM-0013) came before ITEM-0014 and ITEM-0015 added `tools/fetch.sh`,
    `tools/update.sh`, `tools/tools.mk`, `tools/projctl/makefile.go`, the
    reworked Makefile and the staged CI files. Review those, and fix any
    findings under a new item.
  - **Then close:**
    - tick the two remaining M00 criteria (the GitLab pipeline, and raw
      rendering on GitLab);
    - run the `close-milestone` skill;
    - hand over. The user creates `main` with
      `git branch main m00-foundation`, pushes it to both remotes, and sets
      it as the default branch. This is a fast-forward, which keeps the
      per-item commits.
  - **Open on the user's side:** runner and node sizing (M00 verification log,
    and the advice in this session). The lint pipeline now needs under 200 MB
    per job, but M1's container lab will need much more ephemeral storage.
  - **After the merge,** M01 starts with a plan-mode design session.
    13 open questions are needed by M1, plus Q-025; the board groups them.
- 2026-09-25: **Update at session end.** The user reported that the pipeline
  run on `1a68497` passed on the GitLab runners with no change to pod disk
  size, so the M00 criterion for GitLab CI is ticked. Still open for M00
  close:
  - confirm the GitHub pipeline result, which wasn't reported separately;
  - the raw-rendering check in GitLab's file view (the NOTE alert in
    `docs/_index.md`, the Mermaid diagram in ADR-0007);
  - re-run `/code-review high` on the ITEM-0014 and ITEM-0015 code;
  - the `close-milestone` skill and the hand-over.

  Commits `b90b756` and later are documentation only, and go up with the
  user's next push.

