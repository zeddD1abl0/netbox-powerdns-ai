---
id: ITEM-0005
title: projctl — scaffold, index and lint the project tracking files
type: feature
status: done
milestone: M00
requirements: [REQ-012, REQ-025]
depends_on: [ITEM-0004]
created: 2026-09-25
closed: 2026-09-25
---

# ITEM-0005: projctl — scaffold, index and lint the project tracking files

## Goal

Build a small Go tool in `tools/projctl` that makes the tracking model in
ADR-0002 mechanical, so it can't drift.

## Acceptance criteria

- [x] `projctl new item "<title>"` and `projctl new adr "<title>"` scaffold a file with the next free ID.
- [x] `projctl index` regenerates:
  - `project/README.md`: milestones, items grouped by status, questions still open, and requirement coverage;
  - the ADR table in `docs/adr/_index.md`.
- [x] `projctl lint` fails when:
  - an ID is duplicated;
  - front matter is invalid or a status value is unknown;
  - an item names a milestone, REQ, Q or ITEM that doesn't exist;
  - a milestone is `done` but still has open items;
  - an item is `done` without a `closed` date;
  - a generated file is stale;
  - a relative link or anchor doesn't resolve.
- [x] Table-driven tests cover every lint rule, with fixtures.
- [x] The `new-item`, `new-adr` and `close-milestone` skills call `projctl` (through `make item`, `make adr`, `make project` and `make project-lint`) instead of the manual steps.
- [x] `CLAUDE.md` no longer says the board is maintained by hand.

## Notes

- 2026-09-25: Built as its own module, `tools/projctl` (ADR-0013). Its only
  dependency is `go.yaml.in/yaml/v3`. The Makefile runs it with
  `go -C tools/projctl run . -root $(ROOT)`.
- 2026-09-25: Beyond the planned rules, `lint` also checks:
  - milestone ids and statuses, at most one milestone in progress, and a done
    milestone has a `closed` date;
  - H1 headings match the front matter for items, milestones and ADRs;
  - `supersedes` and `superseded by` agree on both ADRs;
  - ADR requirement and question references exist;
  - each REQ and Q id appears exactly once in `requirements.md`;
  - CI files run only make targets, following YAML aliases;
  - `CLAUDE.md` is 150 lines or fewer.
- 2026-09-25: The fixtures are built in Go (`baseRepo` in `projctl_test.go`),
  not as files, so each case states its change in one line. The golden board is
  `testdata/board.golden.md`; regenerate it with `go test -update`. There are
  26 lint cases, plus tests for links, slugs, the golden board, `new` and CLI
  usage.
- 2026-09-25: The board has no "Updated" date, so regenerating an unchanged
  tree produces no diff. It shows the next item: the first in-progress item,
  else the first open one, that isn't waiting on anything.
