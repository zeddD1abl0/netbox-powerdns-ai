---
id: M04
title: Write path
status: planned # planned | in-progress | done
started:
closed:
---

# M04: Write path

## Goal

Apply NetBox's DNS data to PowerDNS safely, then verify the result.

## Scope (provisional)

- Plan and apply to each group's primary, through the API only
- Catalog zone membership for secondaries (ADR-0007)
- Per-zone drift policies: enforce, report, ignore (ADR-0008)
- Change limits and approvals (Q-013); verification by DNS query after apply (Q-014)
- Sync triggers and brownfield import (Q-054)

## Design, non-goals and acceptance criteria

To be written in this milestone's plan-mode session, before implementation
starts. The milestone's branch is `m04-write-path` (ADR-0010).
