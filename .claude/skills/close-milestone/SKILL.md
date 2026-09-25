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
   summary into the milestone's **Verification log**, with the date. (Until
   ITEM-0006 exists, record the manual checks you ran instead.)
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
   - Update `project/README.md`: the current milestone moves to the next one.
   - Run `go run ./tools/projctl lint` once it exists.
   - Commit these changes on the branch.
8. **Hand over.** Show the user:
   - a clean `git status`;
   - the branch's commit list (`git log --oneline main..HEAD`) and a short
     summary of what the milestone delivered;
   - how to finish: review the branch, merge it into `main` with a merge
     commit (not a squash, which would lose the per-item commits), then push;
   - a reminder that the next milestone won't start until the branch is merged.
