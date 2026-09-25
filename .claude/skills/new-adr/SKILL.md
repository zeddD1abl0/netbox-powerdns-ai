---
name: new-adr
description: Record an architecture decision as a new ADR in docs/adr/. Use when a choice has lasting consequences (a technology, a data model, an API convention, a security posture, a scope boundary), when a Q-nnn question is answered with a design choice, or to supersede an earlier ADR.
---

# New architecture decision record

Rules are in ADR-0001.

1. **Create it:** `make adr TITLE="Short title of the decision"`. This takes
   the next free number, fills in `docs/adr/template.md`, prints the new file's
   path and regenerates the ADR index.
2. Open the new file.
3. **Write the ADR.**
   - State the context as the problem, not the solution.
   - List the real options that were considered, including the ones rejected.
   - Tie the decision outcome to the decision drivers.
   - Under **Consequences**, include the bad ones.
   - Say how the decision is **confirmed**, for example by a lint rule, a
     test, a CI check or a review step.
4. **Fill in the front matter.** Link the `requirements` and `questions`. Set
   `status: proposed` until the user agrees, then `accepted`, and set `date`.
5. **Superseding an ADR:** in the old ADR, change **only** its `status` to
   `superseded by ADR-NNNN`. Never edit anything else in an accepted ADR.
6. **Update the links:**
   - if it supersedes an ADR, set `supersedes: ADR-NNNN` in the new ADR's front
     matter (`make project-lint` checks both sides);
   - if it answers a question, move that question to **Answered** in
     `project/requirements.md` and link the ADR there;
   - run `make project` to regenerate the ADR index.
