---
title: Service endpoints
weight: 50
---

# Service endpoints

`nbpdns serve` serves these endpoints at `server.listen`, `:8080` by default,
over plain HTTP. Each answers `GET` and `HEAD`; another method gets `405`,
and another path `404`. None changes anything.

Under `/api`, every error, those included, is an RFC 9457 problem, as
`application/problem+json`. Every response there carries an `X-Flow-ID`:
the request's own, if it sent a valid one, or a new one. It's the request's
`request_id` in the logs. A request's W3C `traceparent` continues its trace.

> [!WARNING]
> The endpoints have no authentication until M10, and they name your server
> groups, zones, and URLs. Keep the port on a trusted network.

| Path | Answers |
|---|---|
| `/livez` | `200 ok` unless no drift refresh has started for `drift.interval` + `drift.timeout` + 1 minute; then `503`, with when the last one started. |
| `/readyz` | `503` until the first drift refresh has finished, whatever its outcome; then `200 ready`. |
| `/status` | The service's state, as text for a person, or as JSON with `?json=1`. |
| `/metrics` | The metrics, in Prometheus's text format, or in OpenMetrics if the scraper asks for it. The [metrics reference](metrics.md) lists them. |
| `/api/…` | The API, as JSON: the service's status, at `/api/status`; the server groups, at `/api/server-groups`; each group's zones, filtered by state; each zone's changes; and each zone's records as NetBox defines them, at `…/rrsets`. Everything is last-known state. Lists are paged with `limit` and `cursor`. The [API reference](api.md) lists every operation. |
| `/api/openapi.yaml` | The API's OpenAPI 3.1 document, `application/yaml`, which describes every operation under `/api`. |
| `/api/docs` | The API's reference, for a browser: Scalar's, built into nbpdns, reading `/api/openapi.yaml`. Its Content-Security-Policy lets it reach no other host. |

Neither `/livez` nor `/readyz` depends on NetBox or the primaries: their
failures show on `/status`, in the metrics, and in the logs.

## `/status`

Without a query, `/status` answers `text/plain; charset=utf-8`: the same
facts as the JSON, in aligned tables. With `?json=1`, it answers
`application/json`. Neither form ever holds a token, an API key, or an OTLP
header.

Fields are only ever added to the JSON, never removed or renamed, so every
field is always present. A field with nothing to say yet is `null`, such as
`schedule.last_refresh` before the first refresh finishes. Times are in RFC
3339 form, in UTC.

This example is from a service with two server groups, one of which can't
be read:

```json
{
  "version": "v0.4.0",
  "revision": "1cc9b657f1ed27ce0922b6f557110c2269af6d98",
  "started": "2026-10-07T01:00:00Z",
  "uptime_seconds": 3600.5,
  "ready": true,
  "live": true,
  "schedule": {
    "interval_seconds": 300,
    "timeout_seconds": 600,
    "last_refresh": {
      "started": "2026-10-07T01:55:00Z",
      "finished": "2026-10-07T01:55:42.3Z",
      "duration_seconds": 42.3,
      "outcome": "incomplete",
      "error": ""
    },
    "last_complete_refresh": "2026-10-07T01:50:41.9Z",
    "next_refresh": "2026-10-07T02:00:00Z",
    "refreshes": {
      "complete": 10,
      "incomplete": 2,
      "failed": 0
    }
  },
  "netbox": {
    "url": "https://netbox.example.com",
    "up": true,
    "error": ""
  },
  "groups": [
    {
      "name": "site-a",
      "url": "https://pdns-a.example.com:8443",
      "status": "ok",
      "last_success": "2026-10-07T01:55:42.3Z",
      "error": "",
      "counts": {
        "in_sync": 41,
        "drift": 1,
        "missing": 0,
        "inactive_in_netbox": 0,
        "ignored": 2,
        "unmanaged": 3
      },
      "drifted_zones": [
        {
          "zone": "example.com.",
          "state": "drift",
          "changes": 2
        }
      ],
      "problems": 0,
      "warnings": 0
    },
    {
      "name": "site-b",
      "url": "https://pdns-b.example.com:8443",
      "status": "failed",
      "last_success": "2026-10-07T01:50:41.9Z",
      "error": "PowerDNS at https://pdns-b.example.com:8443/ isn't reachable: dial tcp: i/o timeout",
      "counts": {
        "in_sync": 44,
        "drift": 0,
        "missing": 0,
        "inactive_in_netbox": 0,
        "ignored": 0,
        "unmanaged": 0
      },
      "drifted_zones": [],
      "problems": 0,
      "warnings": 0
    }
  ],
  "tracing": {
    "exported": true,
    "endpoint": "https://otel.example.com:4318",
    "protocol": "http/protobuf"
  },
  "webhooks": {
    "enabled": true,
    "delay_seconds": 3,
    "last_event": {
      "received": "2026-10-07T01:59:58.2Z",
      "event": "updated",
      "object_type": "netbox_dns.record",
      "request": {
        "id": "3bd63b08-a526-45f3-a819-aa7c507dd31f",
        "user": "admin"
      }
    },
    "pending": {
      "events": 2,
      "zones": ["_default_/example.org."],
      "full": false,
      "due": "2026-10-07T02:00:01.2Z"
    },
    "last_refresh": {
      "started": "2026-10-07T01:58:03Z",
      "finished": "2026-10-07T01:58:03.4Z",
      "zones": ["_default_/example.com."],
      "full": false,
      "reason": "",
      "outcome": "complete",
      "error": "",
      "events": 3,
      "requests": [
        {
          "id": "25ef5d4e-f592-47ff-8d3a-9f10bb3d3e17",
          "user": "admin"
        }
      ]
    }
  }
}
```

| Field | Type | Description |
|---|---|---|
| `version` | string | The running build's version. |
| `revision` | string | The VCS revision it was built from, if known. |
| `started` | time | When the service started. |
| `uptime_seconds` | number | The time since `started`. |
| `ready` | Boolean | Whether `/readyz` answers `200`. |
| `live` | Boolean | Whether `/livez` answers `200`. |
| `schedule.interval_seconds` | number | `drift.interval`. |
| `schedule.timeout_seconds` | number | `drift.timeout`. |
| `schedule.last_refresh` | object or null | The last refresh that finished, or `null` before the first. |
| `schedule.last_refresh.started` | time | When it started. |
| `schedule.last_refresh.finished` | time | When it finished. |
| `schedule.last_refresh.duration_seconds` | number | How long it took. |
| `schedule.last_refresh.outcome` | string | `complete`; `incomplete`, if a server group couldn't be read; or `failed`, if NetBox couldn't be read, or the refresh took longer than `drift.timeout`. |
| `schedule.last_refresh.error` | string | Why a failed refresh failed: NetBox's error, or the timeout. Empty otherwise; a group's error is in `groups[].error`. |
| `schedule.last_complete_refresh` | time or null | When the last complete refresh finished, or `null` if none has. |
| `schedule.next_refresh` | time or null | When the next refresh starts, or `null` while one runs. |
| `schedule.refreshes.complete` | integer | Refreshes that finished complete since the service started. |
| `schedule.refreshes.incomplete` | integer | Refreshes that finished incomplete. |
| `schedule.refreshes.failed` | integer | Refreshes that failed. |
| `netbox.url` | string | NetBox's URL, `netbox.url`. |
| `netbox.up` | Boolean or null | Whether the last refresh could read NetBox, or `null` before the first. |
| `netbox.error` | string | Why NetBox couldn't be read, or empty. |
| `groups[].name` | string | The server group's name. Groups are in the config file's order. |
| `groups[].url` | string | The group's primary's URL. |
| `groups[].status` | string | `ok` or `failed`, as of the last time a refresh tried the group's primary, or `unknown` before that. A refresh that can't read NetBox doesn't try the primaries. |
| `groups[].last_success` | time or null | When the primary was last read and compared, or `null` if it never was. |
| `groups[].error` | string | Why the primary couldn't be read, or empty. |
| `groups[].counts.in_sync` | integer | The group's zones in sync, as of its last successful read, as are the other counts. |
| `groups[].counts.drift` | integer | Its zones whose RRsets differ. |
| `groups[].counts.missing` | integer | Its active NetBox zones that the primary doesn't have. |
| `groups[].counts.inactive_in_netbox` | integer | Its zones that the primary serves, though they aren't active in NetBox. |
| `groups[].counts.ignored` | integer | Its zones with the drift policy `ignore`. |
| `groups[].counts.unmanaged` | integer | Zones on the primary that NetBox doesn't assign to the group. |
| `groups[].drifted_zones` | array | The zones that drifted, as of the group's last successful read. |
| `groups[].drifted_zones[].zone` | string | The zone's absolute name. |
| `groups[].drifted_zones[].state` | string | `drift`, `missing`, or `inactive_in_netbox`. |
| `groups[].drifted_zones[].changes` | integer | How many of its RRsets differ. Run `nbpdns drift --zone` for them. |
| `groups[].problems` | integer | Problems in the data that normalization worked around. |
| `groups[].warnings` | integer | Warnings about the configuration or NetBox's zones. |
| `tracing.exported` | Boolean | Whether spans are exported, that is, whether `otlp.endpoint` is set. |
| `tracing.endpoint` | string | `otlp.endpoint`, or empty. |
| `tracing.protocol` | string | `otlp.protocol`. |
| `webhooks.enabled` | Boolean | Whether `netbox.webhook_secret` is set, so that `/api/netbox-events` takes NetBox's webhooks. |
| `webhooks.delay_seconds` | number | `drift.webhook_delay`: how long the zones that webhooks name wait for the webhooks to stop coming. |
| `webhooks.last_event` | object or null | The last event that a signed webhook brought, or `null` before the first. |
| `webhooks.last_event.received` | time | When it came. |
| `webhooks.last_event.event` | string | What happened to the object: `created`, `updated`, or `deleted`. |
| `webhooks.last_event.object_type` | string | The object's type, such as `netbox_dns.record`. |
| `webhooks.last_event.request.id` | string | NetBox's ID for the request that made the change, or empty if no request did. |
| `webhooks.last_event.request.user` | string | The user who made the request, or empty. |
| `webhooks.pending.events` | integer | The webhooks whose refresh waits, or 0. |
| `webhooks.pending.zones` | array | The zones waiting, each as `view/name`, unless a full refresh waits. |
| `webhooks.pending.full` | Boolean | Whether a full refresh waits: for a view's change, a zone or a record that moved, or more than 100 zones. |
| `webhooks.pending.due` | time or null | When the refresh is due, unless the scheduled one comes first: `drift.webhook_delay` after the last webhook, or 30 seconds after the first, whichever is sooner. `null` if nothing waits. |
| `webhooks.last_refresh` | object or null | The last refresh that webhooks asked for, or that covered the zones they named, or `null`. |
| `webhooks.last_refresh.started` | time | When it started. |
| `webhooks.last_refresh.finished` | time | When it finished. |
| `webhooks.last_refresh.zones` | array or null | The zones it refreshed, each as `view/name`, or `null` for a full refresh. |
| `webhooks.last_refresh.full` | Boolean | Whether it was a full refresh. |
| `webhooks.last_refresh.reason` | string | Why it was full, such as `a view was updated`, or empty. |
| `webhooks.last_refresh.outcome` | string | `complete`, `incomplete`, or `failed`, as for `schedule.last_refresh.outcome`. |
| `webhooks.last_refresh.error` | string | Why it failed, or empty. |
| `webhooks.last_refresh.events` | integer | The webhooks it served. |
| `webhooks.last_refresh.requests[].id` | string | The ID of each NetBox request whose changes it refreshed, up to 128. |
| `webhooks.last_refresh.requests[].user` | string | The user who made the request. |
