---
title: Run nbpdns as a service
weight: 40
---

# Run nbpdns as a service

In this tutorial, you run nbpdns as a service in the development lab. It
compares a zone in NetBox with lab-a's PowerDNS server every 10 seconds.
You check its health, read its status page and its metrics, change the zone
on PowerDNS, and watch the next refresh find the drift.

It takes about 15 minutes, most of it the lab's first start.

## Before you start

You need the repository and its
[development prerequisites](../../README.md#development), which include Docker,
and two terminals, each in the repository's root. If the lab is already
running from another tutorial, run `make lab-down` first, to start from an
empty lab.

[Find drift between NetBox and PowerDNS](find-drift-between-netbox-and-powerdns.md)
explains what nbpdns compares. You don't need to do it first.

## Start the lab and build nbpdns

1. Start the lab:

   ```shell
   make lab-up
   ```

   The lab runs NetBox 4.7 with the NetBox DNS plugin, and a PowerDNS 5.1
   server, the primary of the server group `lab-a`.

2. Build nbpdns:

   ```shell
   make build
   ```

## Put a zone into NetBox and PowerDNS

1. Create the zone `service.example` in NetBox, with three records:

   ```shell
   NETBOX=http://localhost:8047
   ADMIN_TOKEN=nbt_nbpdnslabadm.nbpdnsLabAdminTokenNotForProduction00000
   curl -X POST "$NETBOX/api/plugins/netbox-dns/nameservers/" \
     -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
     -d '{"name": "ns1.service.example"}'
   curl -X POST "$NETBOX/api/plugins/netbox-dns/zones/" \
     -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
     -d '{"name": "service.example", "default_ttl": 3600,
          "nameservers": [{"name": "ns1.service.example"}],
          "soa_mname": {"name": "ns1.service.example"},
          "soa_rname": "hostmaster.service.example",
          "soa_ttl": 3600, "soa_refresh": 10800, "soa_retry": 3600,
          "soa_expire": 604800, "soa_minimum": 3600}'
   curl -X POST "$NETBOX/api/plugins/netbox-dns/records/" \
     -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
     -d '[{"zone": {"name": "service.example"}, "name": "www", "type": "A", "value": "192.0.2.10"},
          {"zone": {"name": "service.example"}, "name": "mail", "type": "A", "value": "192.0.2.25"},
          {"zone": {"name": "service.example"}, "name": "@", "type": "MX", "value": "10 mail"}]'
   ```

2. Create the same zone on lab-a's primary, through PowerDNS's API:

   ```shell
   printf '%s\n' nbpdns-lab-powerdns-api-key-not-for-production > pdns-lab.key
   PDNS=http://localhost:8151/api/v1/servers/localhost
   PDNS_KEY=$(cat pdns-lab.key)
   curl -X POST "$PDNS/zones" \
     -H "X-API-Key: $PDNS_KEY" -H "Content-Type: application/json" \
     -d '{"name": "service.example.", "kind": "Native", "nameservers": [],
          "rrsets": [
            {"name": "service.example.", "type": "SOA", "ttl": 3600,
             "records": [{"content": "ns1.service.example. hostmaster.service.example. 1 10800 3600 604800 3600", "disabled": false}]},
            {"name": "service.example.", "type": "NS", "ttl": 3600,
             "records": [{"content": "ns1.service.example.", "disabled": false}]},
            {"name": "www.service.example.", "type": "A", "ttl": 3600,
             "records": [{"content": "192.0.2.10", "disabled": false}]},
            {"name": "mail.service.example.", "type": "A", "ttl": 3600,
             "records": [{"content": "192.0.2.25", "disabled": false}]},
            {"name": "service.example.", "type": "MX", "ttl": 3600,
             "records": [{"content": "10 mail.service.example.", "disabled": false}]}]}'
   ```

   Both sides now hold the same zone.

## Start the service

1. Create a config file, `nbpdns.yaml`:

   ```yaml
   log:
     format: text
   netbox:
     url: http://localhost:8047
   powerdns:
     groups:
       - name: lab-a
         views: [_default_]
         primary:
           url: http://localhost:8151
           api_key_file: pdns-lab.key
   server:
     listen: 127.0.0.1:8080
   drift:
     interval: 10s
   ```

   `server.listen` is where the service answers, here on this host only.
   `drift.interval` is how often it refreshes the drift report: here every
   10 seconds, the shortest interval nbpdns allows, so that you don't wait.
   The default is 5 minutes.

2. Start the service, with the NetBox token in the environment:

   ```shell
   export NBPDNS_NETBOX_TOKEN=nbt_nbpdnslabadm.nbpdnsLabAdminTokenNotForProduction00000
   bin/nbpdns serve --config nbpdns.yaml
   ```

   ```text
   time=2026-10-07T11:49:23.417+10:00 level=WARN msg="the NetBox URL uses http://, so the token crosses the network unencrypted; use https:// if NetBox offers it" url=http://localhost:8047/ trace_id=73f9… span_id=c713… request_id=Q4A7…
   time=2026-10-07T11:49:23.417+10:00 level=WARN msg="a PowerDNS API URL uses http://, so its API key, which can change every zone, crosses the network unencrypted; put the API behind TLS" group=lab-a url=http://localhost:8151/ trace_id=73f9… span_id=c713… request_id=Q4A7…
   time=2026-10-07T11:49:23.417+10:00 level=INFO msg=serving address=127.0.0.1:8080 interval_seconds=10 groups=1 trace_id=73f9… span_id=c713… request_id=Q4A7…
   time=2026-10-07T11:49:23.543+10:00 level=INFO msg="drift refreshed" group=lab-a in_sync=1 drift=0 missing=0 inactive_in_netbox=0 ignored=0 unmanaged=0 trace_id=5d68… span_id=839b… request_id=7SMH…
   ```

   The lab has no TLS, so nbpdns warns that the token and the API key cross
   the network unencrypted. Then it starts serving, and refreshes at once:
   lab-a has one zone, in sync. A new `drift refreshed` line follows every
   10 seconds. Each refresh has its own `trace_id` and `request_id`.

   Leave the service running, and do the rest in your second terminal.

## Check its health

```shell
curl localhost:8080/readyz
curl localhost:8080/livez
```

```text
ready
ok
```

`/readyz` says the first refresh has finished, and `/livez` that refreshes
keep starting on time. An orchestrator such as Kubernetes uses them to know
when to send traffic, and when to restart nbpdns.

## Read its status

1. See the status page:

   ```shell
   curl localhost:8080/status
   ```

   ```text
   nbpdns v0.0.0-20261007012857-1cc9b657f1ed+dirty, up 3s, since 2026-10-07T01:49:23Z
   Ready: yes. Live: yes.

   Drift refreshes, every 10s, each for up to 10m0s:
     last:          2026-10-07T01:49:23Z, complete, in 100ms
     last complete: 2026-10-07T01:49:23Z
     next:          2026-10-07T01:49:33Z
     finished:      1 complete, 0 incomplete, 0 failed

   NetBox at http://localhost:8047: read by the last refresh

   GROUP  STATUS  LAST SUCCESS          IN SYNC  DRIFT  MISSING  INACTIVE  IGNORED  UNMANAGED  PROBLEMS  WARNINGS  PRIMARY
   lab-a  ok      2026-10-07T01:49:23Z  1        0      0        0         0        0          0         0         http://localhost:8151

   Spans aren't exported; set otlp.endpoint to export them.

   For this page as JSON, add ?json=1. For each RRset's changes, run nbpdns drift.
   ```

   Your version and times differ. The page also opens in a browser, at
   `http://localhost:8080/status`.

2. Ask for the same status as JSON, which scripts can read:

   ```shell
   curl 'localhost:8080/status?json=1'
   ```

   This excerpt shows lab-a's part:

   ```json
   {
     "name": "lab-a",
     "url": "http://localhost:8151",
     "status": "ok",
     "last_success": "2026-10-07T01:49:23.543141989Z",
     "error": "",
     "counts": {
       "in_sync": 1,
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
   ```

## Read its metrics

```shell
curl -s localhost:8080/metrics | grep -E '^nbpdns_(drift_zones|server_group_up|netbox_up)'
```

```text
nbpdns_drift_zones{group="lab-a",state="drift"} 0
nbpdns_drift_zones{group="lab-a",state="ignored"} 0
nbpdns_drift_zones{group="lab-a",state="in_sync"} 1
nbpdns_drift_zones{group="lab-a",state="inactive_in_netbox"} 0
nbpdns_drift_zones{group="lab-a",state="missing"} 0
nbpdns_drift_zones{group="lab-a",state="unmanaged"} 0
nbpdns_netbox_up 1
nbpdns_server_group_up{group="lab-a"} 1
```

These are the metrics Prometheus scrapes: lab-a's zones by state, and
whether the last refresh could read NetBox and lab-a's primary. The full
page has more, such as how long each refresh took, and every request to NetBox
and PowerDNS.

## Make PowerDNS drift

1. Change `www`'s address on lab-a's primary. In this terminal, set the
   variables again first:

   ```shell
   PDNS=http://localhost:8151/api/v1/servers/localhost
   PDNS_KEY=$(cat pdns-lab.key)
   curl -X PATCH "$PDNS/zones/service.example." \
     -H "X-API-Key: $PDNS_KEY" -H "Content-Type: application/json" \
     -d '{"rrsets": [{"name": "www.service.example.", "type": "A", "ttl": 3600, "changetype": "REPLACE",
                      "records": [{"content": "192.0.2.99", "disabled": false}]}]}'
   ```

   NetBox still says `192.0.2.10`.

2. Wait for the next refresh, at most 10 seconds. The service's terminal
   logs it:

   ```text
   time=2026-10-07T11:49:33.617+10:00 level=INFO msg="drift refreshed" group=lab-a in_sync=0 drift=1 missing=0 inactive_in_netbox=0 ignored=0 unmanaged=0 trace_id=07e0… span_id=9785… request_id=SXIV…
   ```

3. See the drifted zone in the metrics:

   ```shell
   curl -s localhost:8080/metrics | grep '^nbpdns_drift_zone_drifted'
   ```

   ```text
   nbpdns_drift_zone_drifted{group="lab-a",state="drift",zone="service.example."} 1
   ```

   There's one such series for each zone that drifted, so an alert on it
   can name the zone. It goes when the zone is back in sync.

4. See it on the status page:

   ```shell
   curl localhost:8080/status
   ```

   The page now ends with a section of drifted zones:

   ```text
   Drifted zones, as of each group's last successful read:
   GROUP  ZONE              STATE  CHANGES
   lab-a  service.example.  drift  1
   ```

5. See the change itself with `nbpdns drift`, which compares once, as each
   refresh does:

   ```shell
   export NBPDNS_NETBOX_TOKEN=nbt_nbpdnslabadm.nbpdnsLabAdminTokenNotForProduction00000
   bin/nbpdns drift --config nbpdns.yaml
   ```

   ```text
   GROUP  STATUS  IN SYNC  DRIFT  MISSING  INACTIVE  IGNORED  UNMANAGED
   lab-a  ok      0        1      0        0         0        0

   Drift:
   GROUP  ZONE              POLICY  CHANGE   NAME                  TYPE  NETBOX           POWERDNS
   lab-a  service.example.  report  changed  www.service.example.  A     3600 192.0.2.10  3600 192.0.2.99
   ```

   Its log lines, on standard error, are left out here.

## Stop the service

In the service's terminal, press Ctrl+C. nbpdns logs `shutting down`, lets
any request in flight finish, and exits with status 0. It does the same on
SIGTERM, which an orchestrator sends.

## Clean up

Remove the lab, with its data, and the files you made:

```shell
make lab-down
rm nbpdns.yaml pdns-lab.key
```

## Next steps

- [Monitor drift with Prometheus](../how-to/monitor-drift-with-prometheus.md)
  scrapes the service and alerts on drift.
- [How nbpdns runs as a service](../explanation/how-nbpdns-runs-as-a-service.md)
  explains the schedule, what the service keeps when NetBox or a primary
  can't be read, and what its health checks mean.
- [Export traces to an OpenTelemetry collector](../how-to/export-traces-to-an-opentelemetry-collector.md)
  sends the trace of each refresh to your tracing backend.
- The [service endpoints reference](../reference/service-endpoints.md)
  lists every field of the status page.
