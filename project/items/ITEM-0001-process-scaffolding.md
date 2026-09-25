---
id: ITEM-0001
title: Process scaffolding (M0a)
type: task
status: done
milestone: M00
requirements: [REQ-020, REQ-022, REQ-025, REQ-026]
depends_on: []
created: 2026-09-25
closed: 2026-09-25
---

# ITEM-0001: Process scaffolding (M0a)

## Goal

Create the files that let any Claude session pick up the project and work the
same way every time. This item adds no Go code, because that needs the module
path from Q-005.

## Acceptance criteria

- [x] `CLAUDE.md`: working rules, workflow, DoD (≤ 150 lines).
- [x] `README.md`: front-door stub.
- [x] `project/brief.md`: the brief verbatim, plus the 2026-09-25 answers.
- [x] `project/requirements.md`: REQ-001 to REQ-026, and Q-001 to Q-051 with defaults.
- [x] `project/milestones/M00-foundation.md`, with the approved plan embedded.
- [x] `project/items/` holding the M0 items.
- [x] `project/README.md`: the board, maintained by hand until ITEM-0005.
- [x] `docs/adr/`: the template and ADR-0001 to ADR-0004.
- [x] `docs/`: the Diátaxis skeleton and the documentation style guide.
- [x] `.claude/settings.json`: permissions, plus a gofmt PostToolUse hook.
- [x] `.claude/skills/`: `new-item`, `new-adr`, `close-milestone`.
- [x] `.gitignore`, `.editorconfig`, `CHANGELOG.md`.

## Notes

- 2026-09-25: The plan said to save the workflow, commit model and
  self-contained rule as Claude memories. They went into `CLAUDE.md` and
  ADR-0003 instead, so they travel with the repo. Memory holds only a
  cross-project profile of the user, which doesn't belong in the repo.
