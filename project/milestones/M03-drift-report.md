---
id: M03
title: Drift report
status: planned # planned | in-progress | done
started:
closed:
---

# M03: Drift report

## Goal

The first safe value: compare NetBox with PowerDNS for each server group and report every difference, without writing anything.

## Scope (provisional)

- Compare each zone in NetBox with each group's primary and secondaries, in memory
- Reports as text and JSON, and exit codes for scripts
- Where each zone's drift policy is set (ADR-0008)
- The scale targets (Q-017), and what happens when NetBox or a server fails (Q-027)

## Design, non-goals and acceptance criteria

To be written in this milestone's plan-mode session, before implementation
starts. The milestone's branch is `m03-drift-report` (ADR-0010).
