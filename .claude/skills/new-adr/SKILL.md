---
name: new-adr
description: Record an architecture decision as a new ADR in docs/adr/. Use when a choice has lasting consequences (a technology, a data model, an API convention, a security posture, a scope boundary), when a Q-nnn question is answered with a design choice, or to supersede an earlier ADR.
---

# New architecture decision record

Rules are in ADR-0001.

> Once `projctl` exists (ITEM-0005), run `go run ./tools/projctl new adr "<title>"`
> and skip to step 3.

1. **Find the next number.** Take the highest `NNNN-*.md` in `docs/adr/` and
   add 1. Numbers are never reused.
2. **Copy the template.** Copy `docs/adr/template.md` to
   `docs/adr/NNNN-short-kebab-title.md`.
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
   - add the ADR to the table in `docs/adr/_index.md` (until `projctl`
     generates it);
   - if it answers a question, move that question to **Answered** in
     `project/requirements.md` and link the ADR there.
