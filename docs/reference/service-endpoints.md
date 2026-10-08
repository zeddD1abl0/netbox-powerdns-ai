---
title: Service endpoints
weight: 50
---

# Service endpoints

`nbpdns serve` serves these endpoints at `server.listen`, `:8080` by default,
over plain HTTP. Each answers `GET` and `HEAD`; another method gets `405`,
and another path `404`. None changes anything.

> [!WARNING]
> The endpoints have no authentication until M10, and they name your server
> groups, zones, and URLs. Keep the port on a trusted network.

| Path | Answers |
|---|---|
| `/livez` | `200 ok` unless no drift refresh has started for `drift.interval` + `drift.timeout` + 1 minute; then `503`, with when the last one started. |
| `/readyz` | `503` until the first drift refresh has finished, whatever its outcome; then `200 ready`. |
| `/status` | The service's state, as text for a person, or as JSON with `?json=1`. |
| `/metrics` | The metrics, in Prometheus's text format, or in OpenMetrics if the scraper asks for it. The [metrics reference](metrics.md) lists them. |
| `/api/status` | The API's view of the service's state, as JSON: what `/status?json=1` shows, without the groups. |
| `/api/openapi.yaml` | The API's OpenAPI 3.1 document, `application/yaml`, which describes every operation under `/api`. |

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
