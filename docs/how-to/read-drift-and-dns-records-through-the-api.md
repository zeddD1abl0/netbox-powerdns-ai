---
title: Read drift and DNS records through the API
weight: 42
---

# Read drift and DNS records through the API

`nbpdns serve` has a read-only API at `/api`. This guide reads it with curl
and jq: the service's state, the zones that drifted in every server group,
a zone's changes, and a zone's records as NetBox defines them, in a form
another tool can take.

The [API reference](../reference/api.md) lists every operation and field.
The service also serves a reference to browse at `/api/docs`.

## Before you start

You need:

- `nbpdns serve` running, reachable at its `server.listen` address;
- curl and jq.

> [!WARNING]
> The API has no authentication until M10, and it names your server groups,
> zones, and records. Reach it only from a trusted network.

Set the service's address once, for the commands below:

```shell
NBPDNS=http://nbpdns.example.com:8080
```

## Check the service

```shell
curl -s "$NBPDNS/api/status" | jq '{ready, last_refresh: .schedule.last_refresh.outcome, netbox_up: .netbox.up}'
```

```json
{
  "ready": true,
  "last_refresh": "complete",
  "netbox_up": true
}
```

Until `ready` is `true`, the first refresh hasn't finished, and no group has
any zones yet.

## List the server groups

```shell
curl -s "$NBPDNS/api/server-groups" |
  jq -r '.items[] | [.name, .status, (.counts.drift // "-"), (.last_success // "never")] | @tsv'
```

```text
site-a	ok	2	2026-10-08T01:10:02.601192065Z
site-b	failed	0	2026-10-08T00:55:02.114376180Z
```

A group that `failed` keeps its last-known state, as of its `last_success`.
Its `error` says why its primary couldn't be read.

## Find the zones that drifted

A group's zones can be filtered by state. The drifted states are `drift`,
`missing`, and `inactive_in_netbox`:

```shell
for g in $(curl -s "$NBPDNS/api/server-groups" | jq -r '.items[].name'); do
  curl -s "$NBPDNS/api/server-groups/$g/zones?state=drift,missing,inactive_in_netbox&limit=1000" |
    jq -r --arg g "$g" '.items[] | [$g, .zone, .state, .change_count] | @tsv'
done
```

```text
site-a	example.com.	drift	2
site-a	parked.example.	inactive_in_netbox	0
```

## See a zone's changes

Give the zone's name with or without its final dot:

```shell
curl -s "$NBPDNS/api/server-groups/site-a/zones/example.com/changes" |
  jq -r '.items[] | [.kind, .name, .type, (.netbox.values // [] | join(",")), (.powerdns.values // [] | join(","))] | @tsv'
```

```text
changed	www.example.com.	A	192.0.2.10	192.0.2.99
extra	old.example.com.	CNAME		www.example.com.
```

Each change is an RRset: `missing` if NetBox has it and the primary doesn't
serve it, `extra` if the primary serves it and NetBox doesn't, and `changed`
if its values or TTL differ.

## Read a zone's records for another tool

A zone's `rrsets` are its records as NetBox defines them, in nbpdns's
normalized form: what nbpdns compares with PowerDNS. This writes them as
zone-file lines, leaving out NetBox's own SOA and NS records, which the
other side has its own of:

```shell
curl -s "$NBPDNS/api/server-groups/site-a/zones/example.com/rrsets?limit=1000" |
  jq -r '.items[] | .name as $n | .type as $t | .ttl as $ttl
    | .records[] | select(.managed | not) | "\($n) \($ttl) IN \($t) \(.value)"'
```

```text
mail.example.com. 300 IN A 192.0.2.25
www.example.com. 300 IN A 192.0.2.10
```

The records are as of NetBox's last successful read, the page's `as_of`,
which is at most `drift.interval` old while NetBox can be read.

## Page through a long list

A list gives up to `limit` items, 1000 at most, and the absolute URL of the
next page in `next`, which is `null` on the last page. Follow it to the end:

```shell
url="$NBPDNS/api/server-groups/site-a/zones/example.com/rrsets?limit=100"
while [ "$url" != null ]; do
  page=$(curl -s "$url")
  echo "$page" | jq -c '.items[]'
  url=$(echo "$page" | jq -r '.next')
done
```

If nbpdns is behind a proxy, set `server.public_url` to the address clients
use, so that `next` points there.

## Follow a request through the logs

Every response carries an `X-Flow-ID`. Send your own to find the request in
nbpdns's logs, where it's the `request_id`:

```shell
curl -s -H "X-Flow-ID: check-1234" "$NBPDNS/api/server-groups/site-z"
```

```json
{"detail":"No server group is named site-z.","instance":"/api/server-groups/site-z","status":404,"title":"Not Found","type":"about:blank"}
```

Every error is a problem like this one, as `application/problem+json`.
