---
name: new-item
description: Create a new work item (feature, bug, debt or task) in project/items/ before starting any piece of work, or when a new problem is found. Use whenever work isn't yet tracked by an ITEM file.
---

# New work item

Every piece of work has an item (ADR-0002). Create it **before** starting the
work.

> Once `projctl` exists (ITEM-0005), run `go run ./tools/projctl new item "<title>"`
> and skip to step 4.

1. **Find the next ID.** Take the highest `ITEM-nnnn` in `project/items/` and
   add 1. Never reuse an ID, even one marked `wontfix`.
2. **Create the file** `project/items/ITEM-nnnn-short-kebab-title.md` from
   [template.md](template.md). Keep the title short and specific.
3. **Fill in the front matter:**
   - `type`: `feature`, `bug`, `debt` or `task`;
   - `status`: `open`, or `blocked` if a dependency isn't met;
   - `milestone`: the milestone it belongs to (`M00`, `M01`, …);
   - `requirements`: the REQ IDs it serves, from `project/requirements.md`;
   - `depends_on`: the ITEM or Q IDs it waits for;
   - `created`: today's date.
4. **Write the goal**, one short paragraph on why, and **checkbox acceptance
   criteria** that someone else could verify.
5. **Add the item to `project/README.md`** in the same change (until `projctl`
   generates it).
6. If the item came from a problem found during other work, add a dated note to
   the item that was being worked on, linking to the new one.

## Working the item

1. **Start.** Make sure you're on the milestone branch (`mNN-short-title`), not
   `main`. Set `status: in-progress`.
2. **Finish.** Tick the acceptance criteria. Set `status: done` and `closed:`
   to the date, or `wontfix` with a note explaining why. Update the board.
3. **Commit** (ADR-0010). Use a Conventional Commit whose last trailers are
   `Refs: ITEM-nnnn`, then the attribution trailer. Checkpoint commits during a
   long item are fine; each one carries the same `Refs` trailer.

**Never move or rename the file.**
