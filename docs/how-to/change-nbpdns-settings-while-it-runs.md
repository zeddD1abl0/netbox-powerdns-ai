---
title: Change nbpdns's settings while it runs
weight: 39
---

# Change nbpdns's settings while it runs

nbpdns's **runtime settings**, such as `drift.interval`, can change while
`nbpdns serve` runs, without a restart: `nbpdns settings set` stores them
in nbpdns's database, and serve applies them within seconds. This guide
changes one, checks that serve uses it, and puts it back.

## Before you start

You need:

- `nbpdns serve` running, with its database at `database.url`;
- a shell as the user that runs `nbpdns serve`, which alone can write the
  database, with the same `database.url`. In a container, run the commands
  in the service's own container, such as with `docker exec nbpdns`.

[Configuration](../reference/configuration.md) marks each key as runtime or
bootstrap. Only runtime keys are settings. A bootstrap key, such as
`database.url` or `server.listen`, comes only from the config file, the
environment or a flag, and takes effect when serve starts.

## List the settings

```shell
nbpdns settings list
```

```text
KEY                           VALUE  SOURCE   STORED
audit.retention_days          2557   default  -
drift.group_concurrency       4      default  -
drift.history_retention_days  400    default  -
drift.interval                5m0s   default  -
drift.timeout                 10m0s  default  -
...
```

`SOURCE` says where each value comes from, and `STORED` what the database
stores for it, if anything.

## Change a setting

1. Store the new value, with a reason for the audit trail:

   ```shell
   nbpdns settings set drift.interval 2m --reason "watching a migration"
   ```

   ```text
   Stored 2m for drift.interval.
   A running nbpdns serve applies it within 5s.
   ```

   nbpdns checks the value as it would the config file's, and refuses one
   that isn't valid, or a bootstrap key.

2. Check that serve uses it. Its status page shows the new interval:

   ```shell
   curl -s 'http://localhost:8080/status?json=1' | jq .schedule.interval_seconds
   ```

   ```text
   120
   ```

   Serve logs `applied the changed settings`, with the keys that changed,
   and counts it in `nbpdns_settings_reloads_total`.

Serve applies `log.level` and the refreshes' timing at once: the next
refresh starts 2 minutes after the last one started. It applies the NetBox
and PowerDNS settings, such as `netbox.timeout`, with new clients, at the
start of the next refresh, and the retentions at the next daily prune.

## If the setting is pinned

If the config file, the environment or a flag sets the key too, that value
wins, and the stored one is kept, but not used:

```text
Stored 2m for drift.interval.
env NBPDNS_DRIFT_INTERVAL pins drift.interval, so nbpdns uses its value until it's unset there.
```

`nbpdns settings list` marks the stored value `(pinned)`. To let the stored
value take effect, remove the key from wherever it's set, and restart
serve, which reads the flags, the environment, and the file only when it
starts.

## Put it back

```shell
nbpdns settings unset drift.interval --reason "migration done"
```

```text
Removed the stored value of drift.interval.
A running nbpdns serve applies it within 5s.
```

The key takes its default again, unless it's pinned.

## See who changed what

Each change is an audit event, with the operating system's user who made
it, and the reason:

```shell
nbpdns audit list --target setting
```

```text
SEQ  TIME                  ACTOR           ACTION         TARGET                  OUTCOME  REASON
312  2026-10-10T03:12:45Z  os-user:jordan  setting.set    setting:drift.interval  success  watching a migration
315  2026-10-10T05:40:02Z  os-user:jordan  setting.unset  setting:drift.interval  success  migration done
```

[Check the audit trail](check-the-audit-trail.md) shows how to read each
event in full: its values before and after the change.

> [!NOTE]
> Until M10, anyone who can write the database can change the settings, so
> keep it readable and writable by nbpdns's user only, as nbpdns creates
> it. A secret, such as `netbox.token`, is stored sealed with a master key:
> see [Protect stored secrets with a master key](protect-stored-secrets-with-a-master-key.md).

## Next steps

- [How nbpdns stores its state](../explanation/how-nbpdns-stores-its-state.md#settings-in-the-database)
  explains how settings are stored and applied.
- [Configuration](../reference/configuration.md) lists every key, and its
  kind.
- [Command line](../reference/command-line.md#nbpdns-settings) describes
  `nbpdns settings`.
