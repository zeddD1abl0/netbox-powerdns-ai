---
id: ITEM-0087
title: 'The audit core: events, chain, retention and nbpdns audit'
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-011, REQ-049, REQ-053]
depends_on: [ITEM-0086]
created: 2026-10-09
closed: 2026-10-10
---

# ITEM-0087: The audit core: events, chain, retention and nbpdns audit

## Goal

The audit trail (ADR-0039): the event model; the action registry and its
generated reference; append, in the same transaction as each change; the
SHA-384 chain; `nbpdns audit list` and `nbpdns audit verify`; and the
daily prune, by `audit.retention_days`, with its anchor.

## Acceptance criteria

- [x] Every action is declared once, and `docs/reference/audit-events.md` is generated from the registry.
- [x] `nbpdns audit verify` passes on a good trail, fails on an edited, deleted or inserted event, and passes after a prune.
- [x] The service's start and stop are events, with the configuration's fingerprint.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-09: Created from M08's approved design, on `plan-m08-m12`.
- 2026-10-10: Built `internal/audit`: the event; its canonical form, a JSON
  object of every field but the hash, in a fixed order, with `before` and
  `after` as strings holding their JSON, so that the hash covers the bytes
  stored, and its SHA-384 (`TestSumIsTheCanonicalForm` pins it, for M17's
  SIEM); the `Verifier`, which reports a gap, an edit, a broken link or an
  event out of order; actors (`os-user:<name>`, or `uid:N` where the system
  names none; `system`; `file:<path>`); and the action registry, from which
  `docs/reference/audit-events.md` is generated. `TestEveryActionIsDeclared`
  parses `actions.go`, so a constant without a declaration fails, and the
  store refuses an undeclared action or a target of the wrong type at run
  time. The registry holds only the actions that exist; each M08 item adds
  its own.
- 2026-10-10: In the store: migration 2, `audit_events` and
  `audit_anchor`; `InTx`, the transaction that a change and its events
  share; `Tx.Audit`, which links each event to the last, or to the anchor;
  `ListAudit`; `VerifyAudit`, in pages of 1000, in one read-only
  transaction, which SQLite begins deferred, so it never blocks serve's
  writes; and `PruneAudit`, which deletes by sequence through the last
  event older than the cutoff, sets the anchor, and records `audit.pruned`
  in the same transaction. Tests break a real trail by editing an event,
  editing one and recomputing its hash, deleting one, deleting the first,
  and forging an anchor, and verify after a prune.
- 2026-10-10: `serve` records `service.started`, with its version and the
  configuration's fingerprint (the SHA-384 of the settings, with their
  sources, and the groups, secrets only as set or not), and
  `service.stopped`, with its uptime and any error, written with a context
  that outlives the signal's. It prunes at its start and then daily, each
  prune its own trace. `audit.retention_days`, default 2557;
  `nbpdns_audit_events_total{action}`; `nbpdns audit list` (filters by
  time, action, actor and target, table or JSON) and `nbpdns audit verify`
  (exit 1 at a break), both refusing a database that isn't migrated. The
  how-to "Check the audit trail". Outcomes are always `success` in M08:
  a change that fails writes nothing, and M10's refused requests will be
  the first failures.
- 2026-10-10: Done. `make check` passed; `make test-integration` passed
  (internal/cli 60.3 s, internal/netbox 10.8 s).
