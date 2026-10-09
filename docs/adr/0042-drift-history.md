---
title: '0042: Drift history'
status: proposed # proposed | accepted | rejected | deprecated | superseded by ADR-NNNN
date: 2026-10-09
decision-makers: [jordan]
requirements: [REQ-044, REQ-046, REQ-052]
questions: []
supersedes: # ADR-NNNN this replaces, if any
---

# 0042: Drift history

## Context and problem statement

`nbpdns serve` keeps each group's last report in memory (ADR-0029). A
restart forgets it, and `/readyz` waits for the first refresh to finish,
which takes about a minute at 1,000 zones. Nothing records when a zone
drifted, or what its changes were then.

On 2026-10-09 the user chose to keep each zone's changes of state and its
RRset changes at each refresh, not only the last state.

## Decision drivers

- What drifted, exactly, at any time can be read back.
- A refresh that changes nothing must not write a row per zone: at a
  five-minute interval and 1,000 zones, that's nearly 300,000 rows a day.
- A restart shouldn't hide the last-known state.
- Retention covers the year that PCI DSS asks for, at least (REQ-053).

## Considered options

1. Changes of state and refreshes only.
2. Those, and the RRset changes at each refresh, zone by zone.
3. Only the last state, restored at start.

## Decision outcome

Chosen: **changes of state and RRset changes**, as the user chose, written
only when they differ.

- **Refreshes:** each refresh, full or of zones, with its trigger, times,
  outcome, error, zones, and NetBox's request IDs, and each group's result
  in it.
- **Zone history:** a zone's row is written when its state, or the digest of
  its RRset changes, differs from its last row, with the changes, the
  serials, the policy and the refresh. A zone that leaves the report gets a
  last row.
- **Restore:** each group's last report is kept whole, replaced at each
  refresh, and read back at start with the outcome of the last refresh. `serve`
  is ready once it has restored, or, with nothing to restore, once its first
  refresh has finished; `/status` says which, and how old the report is. The
  first refresh starts at once either way. This changes when ADR-0029's
  `/readyz` answers: from M08, a restored report counts.
- **Retention:** `drift.history_retention_days`, by default 400. A zone's
  change of state is also an audit event (ADR-0039), kept with the audit
  trail's retention.
- **Reading:** `GET /api/server-groups/{group}/zones/{zone}/history` and
  `GET /api/server-groups/{group}/refreshes`, paged with cursors, and
  `nbpdns drift history`. The API's version becomes 1.2.0.
- NetBox's records, which the API serves at `…/rrsets`, aren't kept: the
  first refresh after a restart reads them again.

### Consequences

- Good: a zone's whole drift, change by change, can be read back.
- Good: a restart serves the last-known state at once.
- Bad: a zone whose changes keep changing writes a row at every refresh.
- Bad: `…/rrsets` has nothing to serve until the first refresh after a
  restart.

### Confirmation

- Tests drive a zone through drift and back, and check its rows, and that a
  refresh with no change writes none.
- An integration test restarts `serve`, and reads the last reports and
  history at once.

## Pros and cons of the options

### Changes of state and refreshes only

- Good: small.
- Bad: what the changes were at the time is lost.

### Only the last state

- Good: smallest.
- Bad: no history at all.

## More information

- ADR-0029 (the service), ADR-0033 (the API), ADR-0038 (the store), ADR-0039
  (audit).
- Recorded in M08's design, 2026-10-09, on `plan-m08-m12` (ADR-0037).
  Implemented by ITEM-0088 and ITEM-0089.
