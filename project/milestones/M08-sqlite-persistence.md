---
id: M08
title: SQLite persistence
status: planned # planned | in-progress | done
started:
closed:
---

# M08: SQLite persistence

## Goal

nbpdns keeps its state across restarts, in an embedded SQLite database.

## Scope (provisional)

- Migrations (Q-026) and the query layer, with a pure-Go SQLite driver (ADR-0009)
- The audit core: the event model, the registry and persistence (Q-035)
- Runtime settings in the database, and who owns each setting (Q-012, Q-024)
- Drift history
- The single-instance lock (ADR-0009)
- Secrets encrypted at rest (Q-023), and the compliance frameworks that shape retention and crypto (Q-036)

## Design, non-goals and acceptance criteria

To be written in this milestone's plan-mode session, before implementation
starts. The milestone's branch is `m08-sqlite-persistence` (ADR-0010).
