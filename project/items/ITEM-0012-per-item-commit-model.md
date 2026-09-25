---
id: ITEM-0012
title: Adopt per-item commits on milestone branches
type: task
status: done
milestone: M00
requirements: [REQ-012, REQ-022]
depends_on: [Q-047]
created: 2026-09-25
closed: 2026-09-25
---

# ITEM-0012: Adopt per-item commits on milestone branches

## Goal

Switch from "the user commits per milestone" to "Claude commits per item on a
milestone branch" (Q-047), and enforce the rule with tooling rather than
relying on memory.

## Acceptance criteria

- [x] ADR-0010 records the model.
- [x] `CLAUDE.md` workflow, gate and Definition of Done updated.
- [x] `.claude/settings.json`: `git commit`, `git add`, `git switch` and `git branch` allowed; `git push` still on ask.
- [x] A PreToolUse guard hook denies `git commit` on `main`. Pipe-tested and confirmed firing live.
- [x] The `new-item` and `close-milestone` skills commit and hand over per the ADR.
- [x] M00 acceptance, ITEM-0011 and the board updated.

## Notes

- 2026-09-25: The guard is a script that reads the command from the hook
  input, not an `if: "Bash(git commit *)"` filter. The script also catches
  compound commands such as `cd … && git commit` and `git -C … commit`.
  Pipe-tested:
  - on `main`, `git commit -m`, a compound `cd && git commit`, and
    `git -C … commit` are all denied;
  - `git status` and `echo git committed` are allowed;
  - on `m00-foundation`, `git commit` is allowed.
