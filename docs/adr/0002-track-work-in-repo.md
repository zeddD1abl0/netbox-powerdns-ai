---
title: "0002: Track work in the repo as small files with generated indexes"
status: accepted
date: 2026-09-25
decision-makers: [jordan]
requirements: [REQ-012, REQ-022, REQ-025, REQ-026]
questions: [Q-003]
---

# 0002: Track work in the repo as small files with generated indexes

## Context and problem statement

The project needs a backlog, milestones and status that a fresh Claude session
can read quickly and keep accurate. The user chose in-repo Markdown over a
forge's issue tracker (Q-003), and the project must not depend on any one forge
(ADR-0003).

go-redbarkwebhook tracked everything in `README.md`, with history split into
`docs/`. It worked, but had two costs:
- The README grew to 76 KB and was read in full every session.
- Archiving meant moving text verbatim between files. That needed its own
  rulebook and a dedicated linter (`docsdrift`).

## Decision drivers

- Fast session start: read a small board, not a large document.
- Traceability from requirement to work item to commit to release (REQ-012).
- No manual archiving step to forget or get wrong.
- Works on any forge, or none.

## Considered options

1. One file per work item with status in front matter, plus a generated board.
2. A single README roadmap with archive files (the go-redbarkwebhook model).
3. A forge issue tracker (rejected by the user in Q-003).

## Decision outcome

Chosen option: **one file per work item, with a generated board**.

```
project/
  README.md          generated board (never hand-edited once projctl exists)
  brief.md           the original brief, append-only
  requirements.md    REQ-nnn and Q-nnn
  milestones/        one file per milestone: Mnn-short-title.md
  items/             one file per work item: ITEM-nnnn-short-title.md
```

- **Item front matter:** `id`, `title`, `type` (feature, bug, debt, task),
  `status` (open, in-progress, blocked, done, wontfix), `milestone`,
  `requirements`, `depends_on` (the ITEM and Q IDs it waits for), `created`,
  `closed`.
- **Item body:** Goal, Acceptance criteria (checkboxes), and Notes (append-only,
  dated).
- **Files never move or get renamed.** Closing an item changes its status.
  Nothing is archived, so there is no archiving rule to enforce.
- **Commits** reference items with a trailer: `Refs: ITEM-nnnn`.
- **`projctl`** (ITEM-0005) is a small Go tool in `tools/`. `new` scaffolds
  items and ADRs, `index` regenerates the board, and `lint` validates everything
  and fails when the board is stale.

### Consequences

- Good: a session reads the board and one or two small files. History is kept
  where it was written, and git shows how it changed.
- Good: the chain from REQ to ITEM to commit to test lives entirely in git.
- Bad: until `projctl` exists, the board is updated by hand.
- Bad: no web UI for the backlog. The forge's file browser is the UI.

### Confirmation

`projctl lint` runs in `make check` and CI.

## More information

- ITEM-0005 builds `projctl`.
