---
id: ITEM-0115
title: Webhooks on any replica, through a queue in the database
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M09
requirements: [REQ-033]
depends_on: [ITEM-0114]
created: 2026-10-18
closed: 2026-11-06
---

# ITEM-0115: Webhooks on any replica, through a queue in the database

## Goal

Webhooks on any replica (ADR-0048, ADR-0036): a refresh queue in the
database (migration `00008_refresh_queue.sql`), which every replica, on
either database, writes a webhook's zones to; the leader gathers it, with
ADR-0036's delay and bounds, and deletes the rows its refresh covered once
that refresh is recorded; and `/status`'s webhooks section from the
database. This lifts M07's "a webhook reaches one replica".

## Acceptance criteria

- [x] A webhook sent to a follower is refreshed by the leader, with ADR-0036's delay and bounds.
- [x] A queued webhook survives a restart, and a takeover.
- [x] `/status`'s webhooks section is the same on every replica.
- [x] The docs on refreshing drift as NetBox changes say that any replica takes webhooks, with a CHANGELOG line.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-18: Created from M09's approved design, at M09's start.
- 2026-11-06: Done. Migration `00008_refresh_queue.sql` adds
  `refresh_queue`, a row for each webhook that asks for a refresh (its
  zones in served views, or a full refresh and its reason, NetBox's request
  and user, and its span's IDs), and `webhook_state`: the queue's mark,
  `covered_through`, the last event, and the last webhook refresh as JSON.
  The `refresh_queue` sequence gives the rows' IDs, and is the webhooks'
  version, which `Versions` reads beside the others; each webhook, and each
  refresh that covers the queue, moves it.
  - Store (`internal/store/queue.go`): `RecordWebhook` writes the event,
    and the row if any, in one `InTx`, on any replica: it's no
    leader-only write. `LoadWebhooks(covered, since)` reads the rows after
    `since`, or, once the mark has moved from `covered`, every row after the
    mark. `RecordRefresh`'s `Queue` deletes the rows up to `Through`, and
    sets the mark and the last refresh, in the refresh's transaction, so a
    refresh that isn't recorded leaves them queued (`TestFence` checks it
    for a follower on PostgreSQL).
  - Service: `hooks` replaces the in-memory batch. `queue` gathers the rows
    after the mark, which `/status` and `nbpdns_drift_pending_zones` show
    on every replica; `pending`, the rows since the leader's last take,
    says when the next zone refresh is due, with ADR-0036's delay from the
    last webhook and 30 s from the first. Every refresh takes the whole
    queue, and more than 100 zones or a full row make it a full one; once
    it's recorded, `cover` drops the rows it took. A refresh that isn't
    recorded leaves its rows queued but no longer due, so a database that
    can't be written isn't met with another refresh at once: the next
    webhook, or the schedule, takes them with the rest. `Notify` writes
    first, then loads the queue back, so its replica shows it at once; it
    returns `(bool, error)`. `Restore` and `Run` load the queue, and `Run`
    retakes it, so a new leader's first refresh covers what an old one
    took. Times are `kept`, so every replica shows the same.
  - The change feed's watcher calls `ReloadWebhooks` when the webhooks'
    version moves, which wakes the leader's `Run`. Each reload is a trace,
    `webhooks reload`.
  - API: a webhook that can't be written is a `503`, a new response,
    `EventNotQueued`, counted as `failed` in `nbpdns_netbox_webhooks_total`
    and as `webhook` in `nbpdns_db_write_failures_total`. The spec, the
    service endpoints reference and the metrics reference now call the
    webhooks section and `nbpdns_drift_pending_zones` the deployment's.
  - No audit event for a webhook: like a refresh, the queue is the
    service's working state, and NetBox's change log records the change.
  - Docs: the webhooks how-to (any replica, the load balancer's address,
    the lag of `database.poll_interval`, `503`, what survives), the
    PostgreSQL how-to, whose warning goes, both explanation pages, and a
    CHANGELOG line.
  - Tests: `TestWebhookQueue` and `TestVersions` on both databases;
    `TestTakeAfterAFailedWrite`, `TestWebhookToAFollower` (delay measured
    from the webhook's arrival), `TestQueuedWebhookSurvivesARestart`,
    `TestQueuedWebhookSurvivesATakeover` (a leader stopped during the zone
    refresh), `TestUnwrittenRefreshLeavesTheWebhooksQueued` and
    `TestWebhookThatCantBeWritten` in the service, on SQLite; the follower
    test compares the webhooks section too; and `TestNetBoxEventNotQueued`
    in the API. `TestReplicasServeTheSameAnswers` sends a record's webhook
    to the follower, and checks that the leader refreshes it and both serve
    the same, webhooks included: it fails without the watcher's reload
    (checked). The takeover end to end is ITEM-0116's case 5.
  - `make check` passes; `make test-integration` passes in 2 min 53 s,
    its largest process at 1.37 GiB. Found on the way: ITEM-0122 (pipeline
    953's lint timeout) and ITEM-0123 (a flaky settings test, older than
    this item).
