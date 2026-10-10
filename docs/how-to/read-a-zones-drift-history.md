---
title: Read a zone's drift history
weight: 44
---

# Read a zone's drift history

`nbpdns serve` keeps a history of each zone's drift in its database: a row
for each refresh that found the zone in another state, or with other
changes, than before. This guide reads a zone's history, and the
refreshes, from the command line, and then through the API.

## Before you start

You need `nbpdns serve` running, or a database it has used, at its
`database.url`. To read from the command line, you need a shell as the
service's user, which alone can read the database. To read through the API,
you need curl, and jq to pick fields out of its JSON.

## Read a zone's history from the command line

```shell
nbpdns drift history --group site-a --zone example.com
```

```text
TIME                  STATE    CHANGES  POLICY  NETBOX SERIAL  POWERDNS SERIAL  REFRESH
2026-10-08T00:10:02Z  in_sync  0        report  2026100801     2026100801       1180
2026-10-08T00:45:01Z  drift    1        report  2026100802     2026100801       1202
2026-10-08T01:10:02Z  in_sync  0        report  2026100803     2026100803       1207
```

Each row holds from its time until the next: here, `example.com` drifted at
00:45, with one RRset that differed, and was back in sync at 01:10. A
refresh that changes nothing writes no row. A zone that left its group's
report, in neither its NetBox views nor its primary, has a last row whose
state is `removed`.

The table counts each row's changes. To see them, ask for JSON:

```shell
nbpdns drift history --group site-a --zone example.com --output json \
  | jq '.[] | select(.state == "drift") | .changes'
```

```json
[
  {
    "name": "www.example.com.",
    "type": "A",
    "kind": "changed",
    "netbox": {"ttl": 300, "values": ["192.0.2.10"]},
    "powerdns": {"ttl": 300, "values": ["192.0.2.99"]}
  }
]
```

`--limit` sets how many of the latest rows to show, 20 by default.

## List the refreshes

Without `--zone`, `nbpdns drift history` lists the latest refreshes, each
with the group's result:

```shell
nbpdns drift history --group site-a --limit 3
```

```text
ID    FINISHED              CAUSE     OUTCOME   GROUP  DRIFTED  ZONES
1205  2026-10-08T01:05:02Z  schedule  failed    -      -        every zone
1206  2026-10-08T01:07:13Z  webhook   complete  ok     21       _default_/example.com.
1207  2026-10-08T01:10:02Z  schedule  complete  ok     20       every zone
```

A refresh that couldn't read NetBox didn't try the group, so it has no
result. A refresh of only some zones, which NetBox's webhooks asked for,
lists them. `--output json` adds the error and the trace ID of each
refresh, and the group's counts of zones by state.

## Read the history through the API

1. Read a zone's history. The API lists it newest first:

   ```shell
   curl -s http://localhost:8080/api/server-groups/site-a/zones/example.com/history \
     | jq '.items[] | {time, state, change_count}'
   ```

   ```json
   {"time": "2026-10-08T01:10:02Z", "state": "in_sync", "change_count": 0}
   {"time": "2026-10-08T00:45:01Z", "state": "drift", "change_count": 1}
   {"time": "2026-10-08T00:10:02Z", "state": "in_sync", "change_count": 0}
   ```

2. Read the refreshes, newest first:

   ```shell
   curl -s 'http://localhost:8080/api/server-groups/site-a/refreshes?limit=10' \
     | jq '.items[] | {id, cause, outcome, status: .group.status}'
   ```

3. Follow each page's `next` link for older rows, until it's `null`.
   [Read drift and DNS records through the API](read-drift-and-dns-records-through-the-api.md#page-through-a-long-list)
   shows how.

## How long the history is kept

`nbpdns serve` deletes refreshes and rows older than
`drift.history_retention_days`, 400 days by default, once a day. It keeps
each zone's last row, however old, since it says what state the zone has
been in since. Each change of a zone's state is also a `zone.drift_changed`
event in the audit trail, which `audit.retention_days` keeps: see
[Check the audit trail](check-the-audit-trail.md).

## Next steps

- [API](../reference/api.md) describes `listZoneHistory` and
  `listRefreshes`, and every field they return.
- [Command line](../reference/command-line.md#nbpdns-drift-history)
  describes `nbpdns drift history`.
