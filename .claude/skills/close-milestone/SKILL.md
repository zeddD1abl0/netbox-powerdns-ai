---
name: close-milestone
description: Run the Definition of Done checks and close a milestone (Mnn) when all its items appear complete, then hand over to the user for the commit. Use when the user asks to finish, close or wrap up a milestone.
---

# Close a milestone

Claude commits on the milestone branch. The user merges into `main`,
pushes, and tags the release (ADR-0010, ADR-0037). The same steps close a
planning branch, whose items are its planning work, and which has no
release of its own. Work through every step in order. If any step fails, stop
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
   - `CHANGELOG.md` has the user-facing changes. Rename `## [Unreleased]` to
     the milestone's release, `## [0.NN.0] - date` (NN without its leading
     zero: `0.8.0` for M08), with a new, empty `## [Unreleased]` above it,
     and check that `go run ./internal/cmd/releasenotes -tag v0.NN.0
     CHANGELOG.md` prints its notes. Once the user judges nbpdns stable, the
     releases are `1.NN.0` (ADR-0037).
   - Any decisions made during the milestone have ADRs.
6. **Manual verification.** Record in the verification log the steps a person
   would take to see the milestone working, and the result when you ran them.
7. **Tracking.**
   - Set the milestone's `status: done` and `closed:` date.
   - Update its GitLab milestone's description, which closes once the branch
     is merged.
   - Set the next milestone's `status: in-progress` only when the user says
     to start it.
   - Run `make project`, then `make project-lint`.
   - Commit these changes on the branch.
8. **Hand over.** Show the user:
   - a clean `git status`;
   - the branch's commit list (`git log --oneline main..HEAD`);
   - the merge request's title and description, written as below, ready to
     paste;
   - the tag to make once it's merged, which publishes the internal release:
     `git tag -a v0.NN.0 -m "nbpdns 0.NN.0"` on `main`'s merge commit, then
     `git push origin v0.NN.0`;
   - how to finish (ADR-0018): push the branch, open a GitLab merge request
     into `main`, and merge it with the **Merge commit** method. Never squash:
     it loses the per-item commits. If GitLab isn't available, merge locally
     with `git merge --no-ff`, then push `main` to both remotes;
   - a reminder that the next milestone won't start until the branch is merged.

   **Title:** `Mnn: <milestone title>`, for example `M01: NetBox read path`.

   **Description:** Markdown, starting with the quick action
   `/milestone %"Mnn: <milestone title>"`, which assigns the GitLab milestone,
   then these sections in this order. Show paths and commands as code, never
   as links: relative links don't resolve in a merge request description.
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
   6. **Merging:** the method (a merge commit, no squash), the tag to make, and
      anything else to do after the merge.

   End it with a `Closes #n` line for each GitLab issue that its items came
   from.

9. **After the merge.** When the user says it's merged, on the next branch:
   record the merge commit and `main`'s pipelines in the verification log,
   tick the last criterion, and close the GitLab milestone.
