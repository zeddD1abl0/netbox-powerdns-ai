---
id: ITEM-0114
title: Every replica serves the API and status from the database
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: [REQ-033, REQ-052]
depends_on: [ITEM-0113]
created: 2026-10-18
closed: 2026-11-05
---

# ITEM-0114: Every replica serves the API and status from the database

## Goal

Every replica serves from the database (ADR-0048): NetBox's zones, the
API's records, stored by the leader zone by zone, when a zone's digest
changes (migration `00007_netbox_zones.sql`); a refresh served once it's
committed, on either database; `Restore` made a reload, at start and
whenever the last refresh moves; `/status`'s deployment facts from the
database, and the replica's own facts its own, the spec saying which; the
drift gauges set on every replica; and a follower ready once it has
loaded the reports.

## Acceptance criteria

- [x] A follower's `/api` answers, records included, match the leader's within the poll interval.
- [x] After a restart, the API serves NetBox's records at once.
- [x] A refresh that can't be written isn't served, on either database, and counts as a write failure.
- [x] `/status`'s deployment facts are the same on every replica, and the spec says which facts are the replica's.
- [x] A refresh that changes one zone writes that zone's row only.
- [x] The docs on monitoring drift show how to aggregate the gauges across replicas, with a CHANGELOG line.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-18: Created from M09's approved design, at M09's start.
- 2026-11-05: Done. Store: migration `00007_netbox_zones.sql`, with
  `netbox_zones` (a row per zone in the groups' views, its JSON, its
  SHA-384 digest, and `refresh_id`, the refresh that wrote it last) and
  `netbox_state`, a seeded singleton (status `up`/`down`/empty, error,
  `zones_as_of`). `RecordRefresh` returns `Recorded` (ID, zone rows,
  NetBox zone rows, events), and writes NetBox's status and zones in the
  refresh's transaction: a row only when a zone's digest changes, deletes
  for zones gone (a zone refresh only within its scope's views and names,
  as `mergeNetBox` has it), and `zones_as_of` on full refreshes
  (`netbox.go`). `RestoreDrift` became `LoadDrift(ctx, since)`: the
  refreshes' version (the `drift_refreshes` sequence), the last *full*
  refresh (the query is now `LastFullRefresh`; restore used to take a zone
  refresh as the last refresh), reports (named by their row, as a rename
  leaves the JSON's name), results, NetBox's state, the zone index, and only
  the zones written after `since`; it returns only the version if that
  hasn't moved. `Versions` has `Refreshes`. `store.Stored(t)` is a time as
  the store keeps it (UTC, microseconds).
  Service: a refresh is worked out (`found`, `foundZones`, without changing
  state), written (`commit` → `keep`), then served (`serve` →
  `applyChanges`, metrics under the lock); a failed write serves nothing,
  counts, and logs "so no replica serves it". With a history, a refresh's
  times, and Run's schedule, are kept at the store's precision, or the
  leader's `/status` differed from a follower's in nanoseconds (found by
  the equivalence test). `Restore` is the start's `reload`; `Reload` loads
  when the version moved past the service's (`Version()`), merging NetBox's
  zones from the index and the zones written since (a full load if memory
  lacks one), and sets the drift gauges; `Run` reloads first, for a new
  leader. `/status`: a follower's `schedule.next_refresh` is the leader's
  row's; `/readyz` is ready once a full refresh is served, with messages
  for a follower and for write failures. CLI: the watcher reloads after
  the groups when `v.Refreshes != svc.Version()`, in a "drift reload" span.
  Spec, `serve`'s help, metrics' help and intro, service endpoints, the
  PostgreSQL how-to (the warning now covers webhooks only), monitoring
  (`max by` across replicas), and both explanations updated; CHANGELOG
  Added, Changed and Fixed lines. Tests: `TestLoadDrift`,
  `TestNetBoxZones` (one changed zone writes one row) on both databases;
  `TestFollowerServesTheLeadersRefreshes`, `TestUnwrittenRefreshIsServedByNoReplica`,
  `TestRestartServesNetBoxsRecords`, `TestHistoryWriteFailure` (not served
  now); `TestReplicasServeTheSameAnswers` on PostgreSQL with the lab (API,
  records, deployment status and gauges; again after a CLI group makes the
  leader refresh; it fails without the watcher's reload, checked);
  `TestServeRestarts` checks the records after a restart. `served.api`
  keeps its contract checker: parsing the spec per call cost the new test
  about 30 s. `make check` passes; `make test-integration` passes in
  2 min 42 s, its largest process at 1.41 GiB. That isn't a slowdown of the
  other tests: on the same lab, HEAD's `internal/cli` integration tests
  took 138 s, and this change's 153 s, the new test's 14 s and noise.
  Found ITEM-0121 (the spec's
  version). The lab was down at the start, and `make lab-up` started it.
