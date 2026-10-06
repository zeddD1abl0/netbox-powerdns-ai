---
name: close-milestone
description: Run the Definition of Done checks and close a milestone (Mnn) when all its items appear complete, then hand over to the user for the commit. Use when the user asks to finish, close or wrap up a milestone.
---

# Close a milestone

Claude commits on the milestone branch. The user merges into `main` and
pushes (ADR-0010). Work through every step in order. If any step fails, stop
and report what's outstanding. Don't mark the milestone done.

1. **Items.** Every item with `milestone: Mnn` is `done` (with a `closed:` date)
   or `wontfix` (with a note). List any that aren't. Every done item has at
   least one commit on the branch with its `Refs` trailer:
   `git log main..HEAD --grep 'ITEM-nnnn'`. Before `main` exists, use
   `git log --grep`.
2. **Acceptance criteria.** Every checkbox in `project/milestones/Mnn-*.md` is
   ticked, and each tick is backed by evidence you can point to.
3. **Checks.** Run `make check`, and `make test-integration` if the milestone
   touched NetBox, PowerDNS or the database. Both must be green. Paste the
   summary into the milestone's **Verification log**, with the date. Forge
   pipeline results come from the user: ask them to push the milestone branch
   and report the GitLab and GitHub results. Claude never pushes.
4. **Reviews.** Run `/code-review high`. Also run `/security-review` if the
   milestone touched authentication, authorisation, audit, secrets or crypto.
   Fix what they find, or record each finding as an item with a reason.
5. **Docs.**
   - Generated references are up to date.
   - The how-to and explanation pages cover the new behaviour.
   - `CHANGELOG.md` has the user-facing changes under `Unreleased`.
   - Any decisions made during the milestone have ADRs.
6. **Manual verification.** Record in the verification log the steps a person
   would take to see the milestone working, and the result when you ran them.
7. **Tracking.**
   - Set the milestone's `status: done` and `closed:` date.
   - Set the next milestone's `status: in-progress` only when the user says
     to start it.
   - Run `make project`, then `make project-lint`.
   - Commit these changes on the branch.
8. **Hand over.** Show the user:
   - a clean `git status`;
   - the branch's commit list (`git log --oneline main..HEAD`);
   - the merge request's title and description, written as below, ready to
     paste;
   - how to finish (ADR-0018): push the branch, open a GitLab merge request
     into `main`, and merge it with the **Merge commit** method. Never squash:
     it loses the per-item commits. If GitLab isn't available, merge locally
     with `git merge --no-ff`, then push `main` to both remotes;
   - a reminder that the next milestone won't start until the branch is merged.

   **Title:** `Mnn: <milestone title>`, for example `M01: NetBox read path`.

   **Description:** Markdown, with these sections in this order. Show paths
   and commands as code, never as links: relative links don't resolve in a
   merge request description.
   1. **Summary:** what the milestone delivers and why, in two or three
      sentences.
   2. **What's included:** each item on one line (`ITEM-nnnn`, its title),
      grouped by phase, then the new ADRs.
   3. **Verification:** the checks and their results: `make check`, the
      integration tests, the reviews, and the manual verification, summarized
      from the milestone's verification log.
   4. **Reviewing:** where to start reading, what deserves the closest look,
      and how to try it locally.
   5. **Known and deferred:** known problems, review findings left unfixed and
      why, and work or questions moved to later milestones.
   6. **Merging:** the method (a merge commit, no squash) and anything to do
      after the merge.
