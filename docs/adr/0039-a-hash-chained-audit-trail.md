---
title: '0039: A hash-chained audit trail'
status: proposed # proposed | accepted | rejected | deprecated | superseded by ADR-NNNN
date: 2026-10-09
decision-makers: [jordan]
requirements: [REQ-011, REQ-049, REQ-053]
questions: [Q-035, Q-036]
supersedes: # ADR-NNNN this replaces, if any
---

# 0039: A hash-chained audit trail

## Context and problem statement

nbpdns must support auditing (REQ-011): every state change is an event,
with its actor, action, target, before and after values, reason, `trace_id`,
source IP and outcome, kept apart from the operational log. ADR-0008 adds
that every drift detection is an event. Until M08 there was nowhere to keep
them.

Q-035 asked what coverage, retention and tamper evidence are needed. Q-036
asked which compliance frameworks apply. On 2026-10-09 the user chose a
hash chain from the first event, and the Essential Eight and the ISM, ISO
27001, and SOC 2 or PCI DSS. The ISM keeps event logs for at least seven
years; PCI DSS for at least one.

## Decision drivers

- Tampering with the trail must show.
- A change and its event must never exist one without the other.
- Retention defaults to the strictest of the frameworks (REQ-053), and stays
  configurable.
- ASD-approved cryptography (REQ-053).
- Events declared once, with their reference generated (CLAUDE.md, principle
  2).
- The SIEM export comes in M16, and must be able to build on this.

## Considered options

1. A hash chain from the first event.
2. Events now, and a hash chain with the SIEM export in M16.
3. No tamper evidence: the SIEM's copy is the only one that's trusted.

## Decision outcome

Chosen: **a hash chain from the first event**, as the user chose.

- **The event:** actor, by type and ID (`os-user:jordan`, `system`,
  `file`); action; target, by type and ID; before and after, as JSON;
  reason; `trace_id` and `request_id`; source IP; outcome; and time.
- **Declared once:** each action is declared in `internal/audit`'s registry,
  with its target type and meaning, and `docs/reference/audit-events.md` is
  generated from it.
- **The chain:** each event stores the SHA-384 of its canonical JSON, which
  includes the hash of the event before it. SHA-384 is ASD-approved. `nbpdns
  audit verify` walks the chain, and fails on a gap or an edit.
- **Atomic:** a change and its event are written in one transaction.
- **Retention:** `audit.retention_days`, by default 2557, seven years. A
  daily prune deletes older events, keeps the hash of the last one it
  deleted as the chain's anchor, and records itself as an event.
- **Reading:** `nbpdns audit list`, filtered by time, action, actor and
  target, as text or JSON. The API serves the trail from M10, behind
  authentication, since it names users and addresses.
- **M08's events:** settings set and unset; groups created, changed and
  removed, by the CLI or the config file's mirror; the master key rotated;
  the trail pruned; the service started, with its configuration's
  fingerprint, and stopped; and a zone's drift state changed (ADR-0008).

### Consequences

- Good: an edited, deleted or inserted event breaks the chain, which
  `verify` reports.
- Good: M16 can export the events in order, with their hashes, so that the
  SIEM can check them too.
- Bad: the chain proves order and integrity, not that nobody rewrote the
  whole trail; an outside copy, from M16, does that.
- Bad: seven years of events take room, though state changes are few.

### Confirmation

- Tests break the chain by editing, deleting and inserting an event, and see
  `verify` fail; and by pruning, and see it pass from the anchor.
- A test checks that every action used in the code is in the registry.
- `generate-check` keeps the events' reference current.

## Pros and cons of the options

### A chain with the SIEM export, in M16

- Good: one piece of work, with the export.
- Bad: the events from M08 to M16 aren't covered.

### No tamper evidence

- Good: simplest.
- Bad: the trail on the host proves nothing until the SIEM has a copy.

## More information

- ADR-0008 (drift policies, and their events), ADR-0038 (the store).
- Recorded in M08's design, 2026-10-09, on `plan-m08-m12` (ADR-0037).
  Implemented by ITEM-0087.
