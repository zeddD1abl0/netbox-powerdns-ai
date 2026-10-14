---
title: '0044: One item per session, and a Haiku agent for read-only searches'
status: accepted # proposed | accepted | rejected | deprecated | superseded by ADR-NNNN
date: 2026-10-14
decision-makers: [jordan]
requirements: [REQ-022] # REQ-nnn this decision serves
questions: [] # Q-nnn this decision answers
supersedes: # ADR-NNNN this replaces, if any
---

# 0044: One item per session, and a Haiku agent for read-only searches

## Context and problem statement

Claude develops nbpdns in Claude Code sessions on Opus 5.5. On 2026-10-14
the user asked how to spend fewer tokens. They'd been advised to send
searches to Haiku 5.5 and simple project work to Sonnet 5.5, keeping Opus
for the code and the orchestration. Opus throughout is acceptable to them;
halving the cost isn't the goal.

The session transcripts in `~/.claude/projects/` record each turn's token
use. Across the project's eight sessions, from 2026-09-25 to 2026-10-14,
at Anthropic's list prices for Opus 5.5:

| What | Tokens | Cost at list prices |
|---|---|---|
| Context read from the prompt cache, every turn | 1,306M | about $261 |
| Output, thinking included | 4.1M | about $82 |
| Context written to the prompt cache | 15.8M | $63 or more |
| All subagents | 27M | about $13 |

Every turn reads the whole conversation again, so each turn costs more
than the one before. One session ran from 2026-10-05 to 2026-10-10,
through the merges of M01 to M07 and into M08: 2,056 turns, compacted four
times, with half its turns above 500k tokens of context and a peak of
967k. It cost about three-quarters of the total. A new session's first
turn reads about 37k tokens, because the project's state lives in
`project/`, not in the conversation. Subagents, mostly the milestones'
security reviews, cost about 3%. The built-in Explore agent ran on Opus
too, since it takes the main conversation's model.

## Decision drivers

- REQ-022: development, done entirely by Claude, stays repeatable.
- Quality comes first. The user accepts Opus throughout, and the code, the
  design and the reviews are where mistakes cost most.
- What counts is the cost of a finished item. A cheaper call that needs
  more turns, or a brief that costs more than the work, saves nothing.
- A long context costs more on every turn, and compaction loses detail.
- A subagent starts cold. It reads its brief, CLAUDE.md and its files
  itself, and only its answer comes back.

## Considered options

1. **Sessions:** as long as the work runs; one session per item.
2. **Models:** Opus throughout; Haiku for searches, Sonnet for project
   work and Opus for the code, the split the user was advised; Opus for
   the work, with a Haiku agent for broad read-only searches.
3. **The Haiku agent:** a project agent of its own, `scout`; a project
   agent named `Explore`, which replaces the built-in one.

## Decision outcome

Chosen: one session per item, and Opus for the work, with a Haiku agent of
its own, `scout`, for broad read-only searches. The user agreed on
2026-10-14.

- **One item per session.** When an item is committed, its notes say
  what's next, as they must before any session ends, and Claude tells the
  user it's a good point to `/clear`. A milestone's design gets a session
  of its own. The next session starts from the board, as CLAUDE.md says.
- **Opus does the work:** the code, the design, the orchestration and the
  reviews, the milestone's closing reviews included. For a routine item,
  lower effort on Opus is the lever to try before a cheaper model, as
  Anthropic's guidance has it. One model also keeps one prompt cache,
  since each model has its own.
- **`scout`,** in `.claude/agents/scout.md`, runs on Haiku 5.5 at low
  effort, with Read, Bash and the LSP, and no tool that edits files; its
  instructions keep Bash to commands that read. Claude sends it a search
  that would read far more than it returns, among files Claude won't edit:
  where something is handled, which files or docs mention something, how
  a flow runs across packages. It answers with `file:line` pointers and a
  short summary. Claude doesn't send it files it's about to change, which
  it must read itself anyway, or a search of a few calls, which costs less
  than a brief.
- **Read narrowly:** the LSP for symbols, line ranges rather than whole
  files, and `git diff --stat` before a diff. Every read stays in the
  context, and is read again on every later turn.
- **Sonnet** is tried once, on ITEM-0094, the M08 docs: a Sonnet subagent
  drafts them against the style guide, and Opus reviews the draft. The
  item records how it compared, and a later ADR decides whether Sonnet
  gets a standing role.

### Consequences

- Good: on the measured history, one item per session would have cut
  about a quarter of the total cost, more than any split between models.
- Good: short sessions aren't compacted, so they don't lose detail.
- Good: broad searches stay out of the main context, and cost little on
  Haiku.
- Bad: the user runs `/clear` between items; Claude can't.
- Bad: each new session pays again for its start, about 37k tokens, and
  reads again the files its item needs.
- Bad: Haiku may miss what Opus would find. Claude reads the lines that a
  finding rests on before relying on it.
- Bad: a scout's brief must stand on its own, since the agent starts
  without the conversation.
- Bad: Bash can change files, so only `scout`'s instructions keep it to
  reading. There are no dedicated search tools to give it instead.

### Confirmation

- CLAUDE.md states the rules, and `.claude/agents/scout.md` defines the
  agent.
- The transcripts show whether sessions stay short: ITEM-0101's notes say
  how it measured them.

## Pros and cons of the options

### Sessions as long as the work runs

- Good: no restarts, and the context holds all the work done so far.
- Bad: every turn reads it all again. The longest session cost
  three-quarters of the total, and was compacted four times.

### One session per item

- Good: the context stays near what one item needs.
- Bad: restarts, and the user's `/clear` between items.

### Opus throughout, with no agents

- Good: one model, one cache, no briefs.
- Bad: broad searches fill the main context, and every later turn pays for
  them.

### Haiku for searches, Sonnet for project work, Opus for the code

- Good: the lowest price per token for each kind of work.
- Bad: project work here is small and woven into the code: an item's front
  matter, a CHANGELOG line, `make project`. A brief costs more than the
  work, and `make` already does the mechanical part.
- Bad: subagents were 3% of the cost, so moving work between models saves
  little beside session length.
- Bad: each model has its own prompt cache.

### A Haiku agent named `Explore`, replacing the built-in one

- Good: nothing new to remember, since Explore is the agent for broad
  searches.
- Bad: it changes every caller of Explore, plugins and built-in skills
  among them, and drops the built-in agent's own instructions.

## More information

- ITEM-0101 made the agent and the CLAUDE.md rules, and holds the
  measurements.
- Prices per million tokens, from Anthropic's model table on 2026-10-14:
  Opus 5.5, $4 input, $20 output and $0.20 for cache reads; Sonnet 5.5, $2
  and $10; Haiku 5.5, $0.10 and $0.50 for prompts up to 100k tokens, and
  five times that above, so a scout's reads should stay small.
- [Claude Code subagents](https://code.claude.com/docs/en/sub-agents): an
  agent's front matter, and how Explore picks its model.
