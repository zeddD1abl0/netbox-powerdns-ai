---
title: "0008: Drift policy per zone"
status: accepted
date: 2026-09-25
decision-makers: [jordan]
requirements: [REQ-024, REQ-031]
questions: [Q-010]
---

# 0008: Drift policy per zone

## Context and problem statement

NetBox is the source of truth (ADR-0004), but PowerDNS can still change outside
the app: someone edits it through its API, or a zone predates the app. The app
needs a rule for what to do when a primary's contents differ from what NetBox
says.

## Decision drivers

- Adopting existing zones must be safe. Deleting production records on the
  first sync would be an outage.
- Once a zone is managed, NetBox really should win.
- Every decision must be visible and audited.

## Considered options

1. A drift policy per zone.
2. Always enforce.
3. Always report.

## Decision outcome

Chosen option: **a drift policy per zone** (Q-010).

| Policy | On drift |
|---|---|
| `enforce` | Correct PowerDNS to match NetBox: overwrite changed records and delete extra ones, subject to the change-safety limits (Q-013). |
| `report` | Change nothing. Record the drift, alert, and show it in the UI and API. |
| `ignore` | Don't check. Useful for zones delegated elsewhere or being migrated. |

- A zone starts in **`report`** until it's deliberately switched to `enforce`.
  This makes adoption safe by default.
- Every drift detection, and every correction it causes, is an audit event.
- Where the policy is set (in NetBox, for example as a custom field or tag, or
  in this app's configuration) is decided in M3 design.

### Consequences

- Good: existing zones can be brought under management gradually, and nothing
  is deleted without someone opting in.
- Bad: zones left in `report` can drift silently if the alerts are ignored.
  Drift counts are exported as metrics (Q-038) so this is visible.
- Linked open questions:
  - what triggers a sync, and how existing zones are imported (Q-054);
  - the thresholds that stop an `enforce` sync and require approval (Q-013).

### Confirmation

- Integration tests cover all three policies. They include a test that a new
  zone never has records deleted before it's switched to `enforce`.
