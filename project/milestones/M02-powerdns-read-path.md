---
id: M02
title: PowerDNS read path
status: planned # planned | in-progress | done
started:
closed:
---

# M02: PowerDNS read path

## Goal

Read each PowerDNS server's zones and records through its API, and show them in the same normalized form as NetBox's.

## Scope (provisional)

- The PowerDNS Authoritative API client, reading into M01's normalized model (ADR-0006)
- Server groups declared in the config file, each a primary and its secondaries, with zones assigned to a group (ADR-0007)
- How nbpdns reaches PowerDNS (Q-021), and how the API is secured in transit (Q-022)
- A PowerDNS lab: two server groups, each a primary and a secondary, on the supported versions (Q-053)
- `nbpdns powerdns` commands
- Extension points for other DNS backends (Q-039), and declaring resources in the config file (Q-043)

## Design, non-goals and acceptance criteria

To be written in this milestone's plan-mode session, before implementation
starts. The milestone's branch is `m02-powerdns-read-path` (ADR-0010).
