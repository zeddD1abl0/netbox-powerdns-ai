---
id: ITEM-0101
title: Keep sessions to one item, and send read-only searches to a Haiku agent
type: task # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-022]
depends_on: []
created: 2026-10-14
closed: 2026-10-14
---

# ITEM-0101: Keep sessions to one item, and send read-only searches to a Haiku agent

## Goal

ADR-0044, which the user agreed to on 2026-10-14: keep each session to one
item, keep Opus for the work, and send broad read-only searches to a Haiku
agent, `scout`. Measured over the project's sessions, session length, not
the choice of model, drives the cost. This item writes the agent and the
CLAUDE.md rules, and records the measurements behind them.

## Acceptance criteria

- [x] ADR-0044 is accepted, and CLAUDE.md states its rules.
- [x] `.claude/agents/scout.md` defines `scout`: Haiku 5.5 at low effort,
  with Read, Bash and the LSP, and no tool that edits files.
- [x] A test search with `scout`'s instructions, on Haiku, answers with
  `file:line` pointers that check out.
- [x] ITEM-0094 has a note asking for the Sonnet trial.
- [x] `make check` passes.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-14: How the cost was measured. Each transcript in
  `~/.claude/projects/-home-jordan-Repositories-netbox-powerdns-ai/`,
  subagents' included, is JSON lines. Each assistant message carries
  `usage`: `input_tokens`, `cache_creation_input_tokens`,
  `cache_read_input_tokens` and `output_tokens`. A message split into
  several lines repeats its `usage`, so count each `message.id` once. A
  turn's context is the sum of the three input counts. Over the 8 main
  sessions and 21 subagent transcripts, 2026-09-25 to 2026-10-14: 2,837
  main turns read 1,306M tokens from the cache, wrote 15.8M to it and put
  out 4.1M; subagents read 25M and wrote 2M. Every turn was on Opus 5.5,
  the subagents' too. The session of 2026-10-05 to 2026-10-10 had 2,056
  turns, a median context of 500k tokens, a peak of 967k and four
  compactions, and read 1,013M tokens from the cache. A session's first
  turn is about 37k tokens. Tool results: `cat` and `sed` gave 2.8M
  characters over 720 calls, and `git diff` 0.6M over 80.
- 2026-10-14: Agents load when a session starts, so `scout` can't run in
  the session that wrote it: the Agent tool said `Agent type 'scout' not
  found`. Its instructions were tested instead through a general-purpose
  agent on Haiku at low effort.
- 2026-10-14: The test asked how a change to a runtime setting reaches a
  running `serve`, from `nbpdns settings set` to where `serve` applies it.
  It took 5 tool calls, about 50k tokens and 17 seconds, every turn on
  `claude-haiku-5-5`. The answer was right: the CLI command, the store's
  transaction and its version sequence, the 5-second poll in
  `watchSettings`, `reloadSettings` and the atomic pointer, and the
  consumers. Of 12 pointers checked, 11 were exact and one was a line out
  (`storedConfig`, 297 not 298). It listed what it hadn't confirmed rather
  than guess. The next session should run `scout` itself once, to check
  that the agent loads.
- 2026-10-14: CLAUDE.md has a 150-line cap, which `make project-lint`
  checks, and it was at the cap. The rules went in as four lines, with the
  detail left to ADR-0044. To make room, four lines that said things twice
  went: the Commands rows for `make project` and for `make item` and
  `make adr`, which the tracking rules give, and the Commits standard,
  whose Conventional Commits the workflow gives; its "under `Unreleased`"
  moved into the Definition of Done. "Read only what the task needs" now
  covers lines of files too. Vale didn't know "Anthropic" or "subagents",
  so both went into the vocabulary.
- 2026-10-14: Done. `make check` passed in a worktree at HEAD with only
  this item's changes: vet, lint, the tests with `-race`, `govulncheck`,
  gitleaks, then, after the vocabulary and CLAUDE.md fixes, docs-lint,
  api-lint, generate-check and project-lint. The commit leaves out the
  uncommitted ITEM-0092 work.
- 2026-10-14: `scout` loads in a new session, and ran on its own
  definition: 7 tool calls, about 28k tokens and 36 seconds, every turn
  on `claude-haiku-5-5`. It answered two questions on the tooling for
  ITEM-0102 and ITEM-0103: how the Go tools are pinned, and whether
  anything checks that `go.mod` is tidy. Its findings checked out; one
  line number was out (`tool` directives are on line 7, not 5). The
  harness flagged its report as instruction-shaped because it quoted
  `.claude/settings.json`; it held only evidence.
