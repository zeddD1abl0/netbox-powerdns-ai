---
title: "0010: Claude commits per item on milestone branches"
status: accepted
date: 2026-09-25
decision-makers: [jordan]
requirements: [REQ-012, REQ-022]
questions: [Q-047]
---

# 0010: Claude commits per item on milestone branches

## Context and problem statement

M0a carried over the go-redbarkwebhook model, where Claude never commits and
the user makes one commit per milestone. That gives a clean revert point per
milestone. But one milestone becomes one large commit, and the link from each
work item to the exact change that delivered it is lost. The brief requires
traceability (REQ-012), and ADR-0002 already defines a `Refs: ITEM-nnnn`
trailer that only pays off with finer-grained commits.

## Decision drivers

- Each item should map to its own commit (REQ-012).
- The user keeps control of what reaches `main` and the remote.
- A per-milestone revert point is still wanted.

## Considered options

1. The user commits once per milestone (the go-redbarkwebhook model).
2. Claude commits per item on a milestone branch, and the user merges.
3. Claude commits directly to `main`.

## Decision outcome

Chosen option: **Claude commits per item on a milestone branch**. The user chose
it on 2026-09-25 (Q-047).

- **Branches:** one per milestone, named `mNN-short-title` (for example
  `m01-service-skeleton`), created from `main` when the milestone starts.
- **Commits:**
  - Claude commits at least once when an item is done, and may add checkpoint
    commits during long items.
  - Every commit uses Conventional Commits and carries a
    `Refs: ITEM-nnnn[, ITEM-nnnn…]` trailer, plus the attribution trailer the
    tooling specifies.
- **Limits:** Claude never commits to `main`, never merges into `main`, and
  never pushes.
- **Hand-over:**
  - When a milestone is closed, the user reviews the branch, merges it into
    `main`, and pushes.
  - **Squash-merging is discouraged**, because it throws away the one commit
    per item.
  - The merge commit on `main` is the per-milestone revert point.
- **Milestone gate:** milestone N doesn't start until M(N-1)'s branch has been
  merged into `main`.

### Consequences

- Good: `git log --grep 'ITEM-0012'` finds the exact change for an item, which
  completes the REQ → ITEM → commit → test chain.
- Good: the merge commit still gives a revert point per milestone.
- Bad: more commits to review. The `close-milestone` skill lists them with a
  summary to keep review manageable.
- Bad: M0 starts on an unborn `main`. The user creates `main` from
  `m00-foundation` when M0 closes, for example with
  `git branch main m00-foundation`.

### Confirmation

- `.claude/hooks/guard-main-commit.sh` is a PreToolUse hook. It denies any
  `git commit` while the current branch is `main`.
- `.claude/settings.json` allows `git commit` and keeps `git push` on ask.
- The `close-milestone` skill checks that every done item has a commit with its
  `Refs` trailer.

## More information

- This supersedes the commit rule written into `CLAUDE.md` in ITEM-0001. That
  rule was never recorded as an ADR.
