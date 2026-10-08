---
title: Refresh drift as NetBox changes
weight: 43
---

# Refresh drift as NetBox changes

`nbpdns serve` refreshes its drift report every `drift.interval`, 5 minutes
by default. NetBox can also tell it of each change as it happens, with a
webhook. A signed webhook makes `nbpdns serve` compare the zones that
changed within seconds. This guide turns webhooks on in nbpdns, sets them up
in NetBox, checks that they arrive, and fixes the usual problems.

[How nbpdns runs as a service](../explanation/how-nbpdns-runs-as-a-service.md#webhooks-from-netbox)
explains what a webhook refreshes, and when.

## Before you start

You need:

- `nbpdns serve` running, at an address that NetBox can reach;
- NetBox's worker running. NetBox sends webhooks from its background
  worker, `manage.py rqworker`. NetBox's Docker setup runs it as the
  `netbox-worker` service, and a NetBox installed from source as the
  `netbox-rq` service;
- a NetBox user who can add webhooks and event rules;
- a secret for the webhook: a random string of at least 16 characters, such
  as `openssl rand -hex 32` prints.

> [!WARNING]
> The secret signs each webhook, so that nbpdns can refuse any that NetBox
> didn't send. It doesn't hide them. Over `http://`, anyone between NetBox
> and nbpdns can read the events, and send them again, which makes nbpdns
> refresh their zones again. Put nbpdns behind a TLS proxy, and give NetBox
> its `https://` URL.

## Turn webhooks on in nbpdns

1. Save the secret in a file that only nbpdns can read, such as
   `/etc/nbpdns/webhook-secret`.
2. Point `netbox.webhook_secret_file` at it in the config file:

   ```yaml
   netbox:
     webhook_secret_file: /etc/nbpdns/webhook-secret
   ```

   Or set `NBPDNS_NETBOX_WEBHOOK_SECRET_FILE`. The
   [configuration reference](../reference/configuration.md#netboxwebhook_secret)
   lists the other ways.
3. Restart `nbpdns serve`, and check that webhooks are on:

   ```shell
   curl -s "$NBPDNS/api/status" | jq .webhooks.enabled
   ```

   ```json
   true
   ```

Until the secret is set, `/api/netbox-events` answers `404`, and only the
scheduled refreshes run.

## Send NetBox's changes to nbpdns

NetBox needs two objects: a **webhook**, which says where and how to send an
event, and an **event rule**, which says which changes to send.

| Webhook field | Value |
|---|---|
| Name | `nbpdns`, or any other |
| URL | nbpdns's address, with `/api/netbox-events`, such as `https://nbpdns.example.com/api/netbox-events` |
| Body template | empty: nbpdns reads the body that NetBox sends without one |
| Secret | the secret in `netbox.webhook_secret` |
| SSL verification | on |

Keep NetBox's defaults for the others: the method `POST`, and the media
type `application/json`.

| Event rule field | Value |
|---|---|
| Object types | the DNS plugin's view, zone, and record |
| Event types | object created, object updated, and object deleted |
| Action type | webhook |
| Webhook | the webhook you created |

nbpdns ignores events for any other object type, and counts them as
`ignored`.

### In NetBox's web interface

1. Go to **Operations**, then **Webhooks** under **Integrations**, and
   select **Add**. Fill in the webhook's fields from the table, and save it.
2. Go to **Operations**, then **Event Rules** under **Integrations**, and
   select **Add**. Fill in the event rule's fields from the table, choosing
   the webhook you saved, and save it.

### With NetBox's REST API

Set NetBox's address, a token that can add webhooks and event rules, and
nbpdns's URL:

```shell
NETBOX=https://netbox.example.com
NETBOX_TOKEN=nbt_...
NBPDNS_EVENTS=https://nbpdns.example.com/api/netbox-events
```

Create the webhook, and keep its ID:

```shell
HOOK_ID=$(jq -n --arg url "$NBPDNS_EVENTS" --rawfile secret /etc/nbpdns/webhook-secret \
  '{name: "nbpdns", payload_url: $url, http_method: "POST", http_content_type: "application/json",
    secret: ($secret | rtrimstr("\n")), ssl_verification: true}' |
  curl -s -X POST "$NETBOX/api/extras/webhooks/" \
    -H "Authorization: Bearer $NETBOX_TOKEN" -H "Content-Type: application/json" -d @- |
  jq .id)
```

Create the event rule, which sends the DNS plugin's changes to it:

```shell
jq -n --argjson hook "$HOOK_ID" \
  '{name: "nbpdns", object_types: ["netbox_dns.view", "netbox_dns.zone", "netbox_dns.record"],
    event_types: ["object_created", "object_updated", "object_deleted"],
    action_type: "webhook", action_object_type: "extras.webhook", action_object_id: $hook}' |
  curl -s -X POST "$NETBOX/api/extras/event-rules/" \
    -H "Authorization: Bearer $NETBOX_TOKEN" -H "Content-Type: application/json" -d @- |
  jq '{id, name}'
```

### With Terraform

NetBox's Terraform provider, `e-breuninger/netbox`, has no argument for a
webhook's secret, and nbpdns refuses webhooks without a signature. So create
the webhook in NetBox's web interface or with its REST API, as the
preceding sections show, and
manage the event rule with Terraform, given the webhook's ID:

```hcl
variable "nbpdns_webhook_id" {
  description = "The ID of the NetBox webhook that sends events to nbpdns."
  type        = number
}

resource "netbox_event_rule" "nbpdns" {
  name             = "nbpdns"
  content_types    = ["netbox_dns.view", "netbox_dns.zone", "netbox_dns.record"]
  event_types      = ["object_created", "object_updated", "object_deleted"]
  action_type      = "webhook"
  action_object_id = var.nbpdns_webhook_id
}
```

## Check that webhooks arrive

Change a record of a zone that a server group serves, wait a few seconds,
then read the webhooks' state:

```shell
curl -s "$NBPDNS/api/status" | jq '.webhooks | {last_event, last_refresh}'
```

```json
{
  "last_event": {
    "received": "2026-10-08T07:14:37.6Z",
    "event": "updated",
    "object_type": "netbox_dns.record",
    "request": {
      "id": "3bd63b08-a526-45f3-a819-aa7c507dd31f",
      "user": "admin"
    }
  },
  "last_refresh": {
    "started": "2026-10-08T07:14:40.6Z",
    "finished": "2026-10-08T07:14:41Z",
    "zones": ["_default_/example.com."],
    "full": false,
    "reason": null,
    "outcome": "complete",
    "error": null,
    "events": 2,
    "requests": [
      {
        "id": "3bd63b08-a526-45f3-a819-aa7c507dd31f",
        "user": "admin"
      }
    ]
  }
}
```

The zone's drift in the API is now as of that refresh. nbpdns's log has a
line `received a NetBox event` for each event, and `zones refreshed` for
each group, with NetBox's request IDs in `netbox_request_ids`. With
`otlp.endpoint` set, each zone refresh is a trace, `drift zone refresh`,
linked to the spans of the webhooks it served.

`nbpdns_netbox_webhooks_total` counts the webhooks by result, and
`nbpdns_drift_zone_refreshes_total` the zone refreshes by outcome.

## Fix a webhook that doesn't work

NetBox shows each delivery that failed under **System**, then **Background
Tasks**, in the default queue's failed jobs, with nbpdns's answer.

- **`401 Unauthorized`:** the webhook's secret isn't `netbox.webhook_secret`,
  or the webhook has no secret, so NetBox sends no `X-Hook-Signature`. The
  problem's detail says which. Set the same secret on both sides. nbpdns
  logs each refusal, with the sender's address, as `refused a NetBox webhook
  whose signature didn't verify`.
- **`404 Not Found`:** webhooks are off in nbpdns, because
  `netbox.webhook_secret` isn't set, or the webhook's URL doesn't end in
  `/api/netbox-events`.
- **`400 Bad Request`:** the webhook has a body template. Empty it: nbpdns
  reads only the body that NetBox sends without one.
- **No failed jobs, and no `last_event`:** NetBox's worker isn't running,
  or the event rule doesn't select the DNS plugin's object types, or it's
  turned off.
- **`last_event` changes, but no zone refresh:** the zone's view isn't one
  that any server group in nbpdns's configuration serves, so nbpdns ignores
  the event.

A webhook that's lost costs nothing lasting: the next scheduled refresh,
within `drift.interval`, finds the change.
