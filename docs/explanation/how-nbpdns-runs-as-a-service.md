---
title: How nbpdns runs as a service
weight: 18
---

# How nbpdns runs as a service

`nbpdns drift` answers "does PowerDNS serve what NetBox says?" once, when
someone runs it. `nbpdns serve` keeps asking, on a schedule, and keeps the
answer where people, Prometheus, and scripts can see it. This page explains
how it refreshes the report, on a schedule and as NetBox's webhooks tell it
of changes, what it keeps when something fails, what its health checks
mean, and how its metrics are shaped.
[ADR-0029](../adr/0029-run-nbpdns-as-a-service-with-prometheus-metrics-an.md)
and [ADR-0035](../adr/0035-refresh-the-zones-that-netbox-s-webhooks-name.md)
record the decisions.

## A refresh, on a schedule

A **refresh** does exactly what `nbpdns drift` does: it reads NetBox once,
for every server group's views, reads each group's primary, and compares
them ([How nbpdns finds drift](how-nbpdns-finds-drift.md)).

- The first refresh starts as soon as `serve` does. The next starts
  `drift.interval` after the last one *started*, 5 minutes by default.
- A refresh that takes longer than the interval delays the next one, which
  then starts at once, with a warning in the log. Refreshes never overlap,
  so NetBox and the primaries never see two at a time.
- A refresh is stopped after `drift.timeout`, 10 minutes by default. It
  then counts as failed, whatever it was reading when it was stopped.
- Each refresh is its own trace, with its own `request_id` in the logs, so
  the requests and log lines of one refresh can be followed together.
- `serve` makes its clients for NetBox and each primary once, when it
  starts, and reuses them. It checks each server's release once, until a
  check succeeds, not every refresh. A group whose client can't be made,
  such as when its certificate file isn't readable yet, gets another try
  each refresh. Any other change to the config file, or to a key file,
  takes a restart.

## Webhooks from NetBox

With `netbox.webhook_secret` set, NetBox's webhooks tell nbpdns of each
change to the DNS plugin's views, zones, and records.
[Refresh drift as NetBox changes](../how-to/refresh-drift-as-netbox-changes.md)
sets them up. nbpdns refuses a webhook unless its signature is the
HMAC-SHA512 of its body, keyed by the secret, and checks that before it
reads anything in the body.

A change in NetBox rarely comes alone. One API call that creates a record
also updates its zone's SOA serial, and a bulk edit sends thousands of
events. So nbpdns gathers them:

- A record's event names its zone, and a zone's event names the zone, and
  its old name too, if the change renamed it. Each zone is in its NetBox
  view. A zone in a view that no server group serves is left out.
- The zones wait until no webhook has come for `drift.webhook_delay`, 3
  seconds by default, or 30 seconds after the first, whichever is sooner.
- Then a **zone refresh** compares only those zones, in the groups that
  serve their views. It lists them in NetBox in one request, by name, lists
  each primary's zones, and reads the RRsets of only those zones, from
  both sides. Each group's result replaces those zones in its last report,
  and its counts and metrics follow. A zone that neither side has any more
  leaves the report.
- A view's change, a zone moved to another view, a record moved to another
  zone, or more than 100 zones make a full refresh instead. An event names
  where a moved zone or record was only by NetBox's ID, and a view's change
  can change every zone in it.

Zone refreshes and scheduled ones share one loop, so they never overlap. A
scheduled refresh that comes first covers every zone waiting, and a full
refresh that webhooks asked for starts the schedule again.

The scheduled refresh stays, as the safety net. NetBox sends each webhook
once, so one that's lost, while nbpdns restarts or the network fails, is
found only by the next scheduled refresh, within `drift.interval`. A zone
refresh that fails, because NetBox or a primary can't be read, does the
same: its zones wait for the next scheduled refresh, and the groups keep
their last reports, as they do when a scheduled refresh fails.

Each zone refresh is a trace of its own, `drift zone refresh`, linked to the
spans of the webhooks it serves, and it carries NetBox's request IDs and
users, as its log lines do. That's the start of following a change from
NetBox to PowerDNS. The rest, through writes to each server, comes with
M13.

## What it keeps when something fails

A refresh can't always read everything. What nbpdns does then follows from
one idea: a failure to *read* PowerDNS isn't news about what PowerDNS
*serves*.

- **A primary that can't be read:** its group keeps the report from its
  last successful read, and `nbpdns_server_group_up` goes to 0. The other
  groups are compared as usual.
- **NetBox can't be read:** nothing can be compared, so the refresh fails,
  every group keeps its last report, and `nbpdns_netbox_up` goes to 0. The
  primaries aren't tried, so their groups' `up` stays as it was.
- **The refresh runs out of time:** it fails, and the status page says it
  took longer than `drift.timeout`. Every group keeps its last report, and
  neither NetBox's `up` nor any group's changes: a slow read isn't a failed
  one.

Forgetting the last report instead would make an outage look like
everything had come back in sync, and resolve the very drift alerts that
should still be firing. Keeping it, with `up` at 0 and the time of the last
success beside it, says both things at once: here's the last known drift,
and it's getting old.

The state lives in memory. A restart forgets it, and the first refresh
after the restart finds it again. Keeping history across restarts arrives
with the database, in M08.

## Ready and live

Two endpoints tell an orchestrator, such as Kubernetes, how nbpdns is.

- **`/readyz`** answers 200 once the first refresh has finished, whatever
  its outcome. Before that, nbpdns has nothing to report.
- **`/livez`** answers 200 unless no refresh has started for
  `drift.interval` + `drift.timeout` + 1 minute. Since every refresh is
  bounded by its timeout, that only happens if the refresh loop itself is
  stuck, and restarting nbpdns is the right fix.

Neither looks at NetBox or the primaries. If an outage made nbpdns unready,
a scraper that follows readiness could stop collecting the metrics that
show the outage. If it made nbpdns not live, it would be restarted over and
over, which fixes nothing upstream. Upstream failures show in the metrics,
on `/status`, and in the logs instead.

## Metrics, and their cost

The metrics are nbpdns's own. PowerDNS's statistics stay on PowerDNS's own
`/metrics`, and the [metrics reference](../reference/metrics.md) lists every
one of nbpdns's.

They're shaped so that an alert can name the problem, without the number of
series growing with the number of zones:

- **Per server group:** zones by state, RRset changes by kind, problems,
  and warnings. That's about a dozen series per group.
- **Per drifted zone:** one series, `nbpdns_drift_zone_drifted`, labelled
  with the group, the zone, and its state, which disappears once the zone
  is back in sync. An alert on it names the zone, and the series grow only
  with drift.
- **Per refresh:** outcomes, durations, and the time of the last refresh
  and the last complete one.
- **Per request:** counts by status, durations, and retries, for NetBox and
  for each primary.

A series for every zone would give dashboards per zone, but at 1,000 zones
that's about 6,000 series, nearly all of them always zero.
[Monitor drift with Prometheus](../how-to/monitor-drift-with-prometheus.md)
gives alert rules that use them.

## The status page

`/status` shows the same state for people: the schedule, how the last
refresh went, NetBox's and each group's state, the drifted zones, and
NetBox's webhooks: the last event, what waits, and the last refresh they
asked for. With
`?json=1`, it's JSON for scripts. Its fields are only ever added, never
removed, so a script that reads it keeps working.

It isn't the drift report. Each zone's RRset changes are in the API, at
`/api/server-groups/{group}/zones/{zone}/changes`, and `nbpdns drift` shows
them too. The [service endpoints reference](../reference/service-endpoints.md)
lists the status page's fields.

## Who can see it

The listener at `server.listen` has no authentication until M10. That's
acceptable because nothing it serves changes anything. The one thing it
takes, NetBox's webhooks, needs NetBox's signature, and only makes nbpdns
read again. But it isn't private: the pages name your server groups, zones, and URLs, and a group's
error can name a host and port. They never hold a token, a key, or an OTLP
header.

So keep the port on a trusted network: bind it to `127.0.0.1` when only
the host scrapes it, or restrict it with a firewall or a network policy.

## What it doesn't do yet

- **Correct drift.** Nothing is written to PowerDNS until M13.
- **Remember across restarts.** That comes with the database, in M08.
- **Run more than one instance.** High availability comes in M09.
