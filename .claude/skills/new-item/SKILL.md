---
name: new-item
description: Create a new work item (feature, bug, debt or task) in project/items/ before starting any piece of work, or when a new problem is found. Use whenever work isn't yet tracked by an ITEM file.
---

# New work item

Every piece of work has an item (ADR-0002). Create it **before** starting the
work.

1. **Create it:**
   `make item TITLE="Short, specific title" TYPE=feature|bug|debt|task [MILESTONE=Mnn]`.
   This takes the next free ID, fills in [template.md](template.md), prints the
   new file's path and regenerates the board. The milestone defaults to the one
   in progress.
2. **Fill in the front matter:**
   - `requirements`: the REQ IDs it serves, from `project/requirements.md`;
   - `depends_on`: the ITEM or Q IDs it waits for;
   - `status: blocked` if a dependency isn't met.
3. **Write the goal**, one short paragraph on why, and **checkbox acceptance
   criteria** that someone else could verify.
4. Run `make project` to refresh the board, then `make project-lint`.
5. If the item came from a problem found during other work, add a dated note to
   the item that was being worked on, linking to the new one.

## Working the item

1. **Start.** Make sure you're on the milestone branch (`mNN-short-title`), not
   `main`. Set `status: in-progress`.
2. **Finish.** Tick the acceptance criteria. Set `status: done` and `closed:`
   to the date, or `wontfix` with a note explaining why. Run `make project`.
3. **Commit** (ADR-0010). Use a Conventional Commit whose last trailers are
   `Refs: ITEM-nnnn`, then the attribution trailer. Checkpoint commits during a
   long item are fine; each one carries the same `Refs` trailer.

**Never move or rename the file.**
