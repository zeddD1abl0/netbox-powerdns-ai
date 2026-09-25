---
name: close-milestone
description: Run the Definition of Done checks and close a milestone (Mnn) when all its items appear complete, then hand over to the user for the commit. Use when the user asks to finish, close or wrap up a milestone.
---

# Close a milestone

The user commits; this skill never does. Work through every step in order. If
any step fails, stop and report what's outstanding. Don't mark the milestone
done.

1. **Items.** Every item with `milestone: Mnn` is `done` (with a `closed:` date)
   or `wontfix` (with a note). List any that aren't.
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
8. **Hand over.** Show the user:
   - `git status`;
   - a Conventional Commit message for the milestone, with a body summarising
     it and `Refs:` trailers for its items;
   - a reminder that the next milestone won't start until this one is
     committed.
