---
id: M06
title: NetBox webhooks
status: planned # planned | in-progress | done
started:
closed:
---

# M06: NetBox webhooks

## Goal

React to changes in NetBox as they happen, instead of waiting for the next scheduled refresh.

## Scope (provisional)

- Receive NetBox event-rule webhooks and check their HMAC signature
- Refresh only the affected zones (the trigger part of Q-054)
- Carry NetBox's request ID and user into the trace (Q-037)

## Design, non-goals and acceptance criteria

To be written in this milestone's plan-mode session, before implementation
starts. The milestone's branch is `m06-netbox-webhooks` (ADR-0010).
