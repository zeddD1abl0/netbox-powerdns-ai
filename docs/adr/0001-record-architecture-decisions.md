---
title: "0001: Record architecture decisions"
status: accepted
date: 2026-09-25
decision-makers: [jordan]
requirements: [REQ-020, REQ-022]
questions: []
---

# 0001: Record architecture decisions

## Context and problem statement

Claude develops this project across many sessions, and each session starts
with no memory of earlier ones. A decision that lives only in a conversation is
lost when the session ends, and a later session may undo it without knowing why
it was made.

In go-redbarkwebhook, decisions went into a single `docs/decisions.md`. It grew
to 104 KB, and had to be read to find any one decision.

## Decision drivers

- A fresh session must be able to find the rationale for a choice quickly.
- Decisions must not be silently rewritten after the fact (traceability,
  REQ-012).
- The record must live in the repo (REQ-025, REQ-026).

## Considered options

1. One ADR file per decision, in MADR format, under `docs/adr/`.
2. A single running decisions log.
3. Decisions recorded only in milestone files.

## Decision outcome

Chosen option: **one ADR file per decision (MADR 4.0)**. A session reads only
the ADR it needs, and each file keeps a stable link.

Rules:
- Name files `NNNN-short-title.md`. Numbers are sequential and never reused.
- Start from [`template.md`](template.md).
- An ADR starts as `proposed`. It becomes `accepted` when the user agrees,
  either explicitly or by approving a plan that contains it.
- **An accepted ADR is never edited.** To change a decision, write a new ADR and
  set the old one's status to `superseded by ADR-NNNN`. The status line is the
  only change allowed to an accepted ADR.
- Front matter lists the requirements an ADR serves and the questions it
  answers, for traceability.
- ADRs are published with the documentation, as part of the explanation
  material.

### Consequences

- Good: small files, stable links, and a clear history of what was believed and
  when.
- Bad: more files. Finding the ADRs on a topic relies on the index, which
  `projctl` generates (ITEM-0005).

### Confirmation

`projctl lint` checks ADR front matter, numbering and status values.

## More information

- MADR: <https://adr.github.io/madr/>
