---
title: Check the audit trail
weight: 47
---

# Check the audit trail

nbpdns records every change of its state as an event in its audit trail,
in its database. This guide checks that no event was changed, removed or
inserted, lists the events you want, and reads one event in full.

## Before you start

You need:

- a database that `nbpdns serve` has used, at its `database.url`;
- a shell as the user that runs `nbpdns serve`, since only that user can
  read the database.

Run each command with the same `database.url` as the service, from its
config file or the environment. In a container, run them in the
service's own container:

```shell
docker exec nbpdns nbpdns audit verify
```

The commands only read the trail, so they're safe while the service runs.

## Verify the trail

```shell
nbpdns audit verify
```

```text
The audit trail is intact: 214 events, from event 1, at 2026-10-10T02:15:09Z, to event 214, at 2026-10-12T09:41:27Z.
```

`audit verify` walks every event, in order, and checks the SHA-384 hash
that each one holds of its contents and of the event before it. If an
event was changed, removed or inserted, it stops there, and exits with
status 1:

```text
nbpdns: the audit trail is broken at event 37: its contents don't match its hash, so it was changed
```

Once `nbpdns serve` has deleted events older than `audit.retention_days`,
the trail starts after the last one it deleted, and `audit verify` says
so.

> [!IMPORTANT]
> The chain can't show that the latest events were removed, or that the
> whole trail was rewritten, by someone who can write the database file. A
> copy of the trail kept elsewhere can. From M17, nbpdns sends its events
> to a SIEM.

## List events

`nbpdns audit list` lists the latest 100 events, oldest first:

```shell
nbpdns audit list
```

```text
SEQ  TIME                  ACTOR   ACTION           TARGET            OUTCOME  REASON
212  2026-10-12T09:40:51Z  system  service.stopped  service:nbpdns-1  success  -
213  2026-10-12T09:41:27Z  system  service.started  service:nbpdns-1  success  -
214  2026-10-12T09:41:27Z  system  audit.pruned     audit:trail       success  -
```

Choose the events with flags, which combine:

| To list | Use |
|---|---|
| events since a time, or within a time | `--since 2026-10-01T00:00:00Z`, or `--since 24h` |
| events before a time | `--until 2026-10-02T00:00:00Z` |
| one action's events | `--action service.started` |
| one actor's events | `--actor system`, or `--actor os-user:jordan` |
| the events about a target, or a type of target | `--target service:nbpdns-1`, or `--target service` |
| more or fewer events | `--limit 1000` |

[Audit events](../reference/audit-events.md) lists every action, and what
its events hold.

## Read an event in full

The table leaves out each event's values before and after the change, and
its hashes. `--output json` includes them:

```shell
nbpdns audit list --action service.started --limit 1 --output json
```

```json
[
  {
    "seq": 213,
    "time": "2026-10-12T09:41:27.501942Z",
    "actor": "system",
    "action": "service.started",
    "target": "service:nbpdns-1",
    "before": null,
    "after": {
      "config_fingerprint": "sha384:5e3b…",
      "version": "v0.8.0"
    },
    "reason": "",
    "outcome": "success",
    "trace_id": "8c2f…",
    "request_id": "QH6T…",
    "source_ip": "",
    "prev_hash": "0b7d…",
    "hash": "a41c…"
  }
]
```

The `trace_id` and `request_id` are those of the service's log lines and
trace for the same command, so you can find what else it did. A
`config_fingerprint` that differs from the one before it means the
configuration changed between the two starts.

## Keep events for longer, or less long

`nbpdns serve` deletes events older than `audit.retention_days` once a day,
and records that it did, as an `audit.pruned` event. The default, 2557
days, is seven years, the minimum that the ISM sets for event logs. To change it, set
`audit.retention_days` in the config file, and restart the service.

> [!WARNING]
> The events it deletes are gone. Copy the database first if you shorten
> the retention, and check that the new one meets your compliance
> obligations.

## Next steps

- [Audit events](../reference/audit-events.md) describes every event's
  fields, and each action.
- [Configuration](../reference/configuration.md#auditretention_days)
  describes `audit.retention_days`.
