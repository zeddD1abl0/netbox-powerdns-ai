---
title: "0018: Merge milestones through GitLab merge requests"
status: accepted
date: 2026-09-29
decision-makers: [jordan]
requirements: [REQ-012, REQ-026]
questions: []
supersedes:
---

# 0018: Merge milestones through GitLab merge requests

## Context and problem statement

ADR-0010 has Claude commit each item on a milestone branch, and the user merge
that branch into `main` with a merge commit, which becomes the milestone's
revert point. It doesn't say where or how the merge happens.

M00 was merged through a GitLab merge request on 2026-09-26. The project's
merge method was fast-forward, so GitLab moved `main` to the branch tip
(`fcbdaeb`) and made no merge commit.

The repository has two remotes: GitLab (`origin`), where the user reviews and
merges, and GitHub (`github`), which mirrors it. Every milestone should merge
the same way, and the process mustn't depend on GitLab for anything that
can't also be done with git alone.

## Decision drivers

- One merge commit per milestone on `main`, as its revert point (ADR-0010).
- Keep the per-item commits, so each item stays traceable to its change
  (REQ-012).
- Review in a merge-request view, with the CI results next to the diff.
- Don't depend on one forge (REQ-026): merging must also work without GitLab.

## Considered options

1. A GitLab merge request with the **Merge commit** method.
2. A GitLab merge request with **Merge commit with semi-linear history**.
3. A GitLab merge request with **Fast-forward merge**.
4. Squashing the branch when merging, with any method.
5. No merge request: the user merges locally with `git merge --no-ff`.

## Decision outcome

Chosen option: **a GitLab merge request with the Merge commit method**. It
always creates the merge commit ADR-0010 needs and keeps every item's commit,
with review and CI in one place. The user approved it in the M01 design on
2026-09-27.

- **GitLab is the primary forge.** The user pushes the milestone branch, opens
  a merge request into `main`, and merges it there.
- **Project settings** (GitLab: Settings, then Merge requests):
  - merge method: **Merge commit**;
  - squash commits when merging: **Do not allow**.

  The user confirmed both on 2026-09-29.
- **The merge commit is the milestone's revert point.**
  `git log --first-parent main` lists one merge per milestone.
- **GitHub is a push mirror** of every branch, configured in GitLab. Nothing is
  merged on GitHub.
- **Fallback,** if GitLab isn't available: merge locally with
  `git merge --no-ff`, then push `main` to both remotes. The result is the same
  merge commit.
- **The merge request's title and description** come from the
  `close-milestone` skill, in fixed sections.
- **M00 is the exception.** It was fast-forwarded before this rule existed, so
  it has no merge commit. Its last commit, `fcbdaeb`, marks where it ends.

### Consequences

- Good: from M01 on, each milestone is one merge commit on `main`, which
  `git revert -m 1` can undo as a whole.
- Good: the per-item commits and their `Refs` trailers stay in `main`'s
  history.
- Good: the fallback needs only git, so the process survives a move away from
  GitLab.
- Bad: the merge method is a GitLab project setting, outside the repository.
  Nothing in the repository notices if it changes, so the `close-milestone`
  hand-over names the method every time.
- Bad: `main`'s history isn't linear. `git log --first-parent main` gives the
  milestone view.

### Confirmation

- From M01 on, `git log --merges --first-parent main` shows exactly one merge
  commit per closed milestone.
- The `close-milestone` skill's hand-over writes the merge request's title and
  description, and names the merge method.

## Pros and cons of the options

### Merge commit

- Good: always creates a merge commit, even when a fast-forward was possible.
- Bad: depends on a project setting outside the repository.

### Merge commit with semi-linear history

- Good: creates a merge commit, and keeps the history close to linear.
- Bad: merging needs the branch to be up to date with `main`. If it isn't, the
  branch is rebased, which rewrites the commits that were reviewed.

### Fast-forward merge

- Good: linear history.
- Bad: never creates a merge commit, so there's no revert point. M00 was
  merged this way.

### Squash

- Good: one tidy commit per milestone.
- Bad: throws away the per-item commits and their `Refs` trailers (ADR-0010).

### Local merge only

- Good: needs only git.
- Bad: there's no review view, and the CI results aren't shown next to the
  change.

## More information

- [ADR-0010](0010-claude-commits-per-item-on-milestone-branches.md): Claude
  commits per item on milestone branches.
- The M00 merge is recorded in `project/milestones/M00-foundation.md`.
- Recorded by ITEM-0018.
