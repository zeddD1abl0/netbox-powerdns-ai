---
id: M09
title: PostgreSQL and HA
status: planned # planned | in-progress | done
started:
closed:
---

# M09: PostgreSQL and HA

## Goal

Run several replicas on PostgreSQL, with exactly one active sync worker.

## Scope (provisional)

- PostgreSQL, with the same migrations as SQLite (ADR-0009)
- Leader election for the worker (ADR-0009)
- Replica tests: only one replica runs the worker, and another takes over when it stops

## Design, non-goals and acceptance criteria

To be written in this milestone's plan-mode session, before implementation
starts. The milestone's branch is `m09-postgresql-and-ha` (ADR-0010).
