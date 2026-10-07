---
title: Monitor drift with Prometheus
weight: 40
---

# Monitor drift with Prometheus

`nbpdns serve` exposes its metrics at `/metrics`. This guide has Prometheus
scrape them, and alert when a zone drifts, when nbpdns can't read NetBox or
a primary, and when nbpdns stops refreshing.

## Before you start

You need:

- `nbpdns serve` running, reachable from Prometheus at its `server.listen`
  address, `:8080` by default;
- Prometheus 2.x or 3.x, and its `promtool`.

> [!WARNING]
> The listener has no authentication until M10, and its metrics name your
> server groups and zones. Let only Prometheus reach it.

## Scrape nbpdns

1. Add a job to `prometheus.yml`:

   ```yaml
   scrape_configs:
     - job_name: nbpdns
       static_configs:
         - targets: ["nbpdns.example.com:8080"]
   ```

   The default scrape interval, a minute, suits nbpdns: its numbers change
   once a refresh, every `drift.interval`.

2. Reload Prometheus, and check that the target is up, at **Status >
   Targets** in its web UI, or with this query:

   ```text
   up{job="nbpdns"}
   ```

3. See each group's zones by state:

   ```text
   sum by (group, state) (nbpdns_drift_zones)
   ```

## Alert on drift and failures

1. Save these rules as `nbpdns-rules.yml`, beside `prometheus.yml`:

   ```yaml
   groups:
     - name: nbpdns
       rules:
         - alert: NbpdnsZoneDrifted
           expr: nbpdns_drift_zone_drifted == 1
           for: 15m
           labels:
             severity: warning
           annotations:
             summary: "Zone {{ $labels.zone }} drifted in server group {{ $labels.group }}"
             description: >-
               PowerDNS doesn't serve what NetBox says: the zone's state is
               {{ $labels.state }}. Run nbpdns drift --group {{ $labels.group }}
               --zone {{ $labels.zone }} to see the changes.
         - alert: NbpdnsServerGroupDown
           expr: nbpdns_server_group_up == 0
           for: 10m
           labels:
             severity: warning
           annotations:
             summary: "nbpdns can't read the primary of server group {{ $labels.group }}"
             description: The group's drift is as of its last successful read. Its error is on nbpdns's /status page.
         - alert: NbpdnsNetBoxDown
           expr: nbpdns_netbox_up == 0
           for: 10m
           labels:
             severity: warning
           annotations:
             summary: nbpdns can't read NetBox
             description: No drift is compared until it can. The error is on nbpdns's /status page.
         - alert: NbpdnsNoCompleteRefresh
           expr: time() - nbpdns_drift_last_complete_refresh_timestamp_seconds > 3 * 300
           for: 5m
           labels:
             severity: warning
           annotations:
             summary: nbpdns hasn't compared every server group for three intervals
         - alert: NbpdnsDown
           expr: up{job="nbpdns"} == 0
           for: 5m
           labels:
             severity: critical
           annotations:
             summary: Prometheus can't scrape nbpdns
   ```

   - **`NbpdnsZoneDrifted`** fires once per drifted zone, naming it. The
     `for: 15m` lets a change that nbpdns sees halfway through settle: with
     the default interval, it fires after three refreshes in a row find the
     drift.
   - **`NbpdnsServerGroupDown`** and **`NbpdnsNetBoxDown`** mean the drift
     shown is getting old. A group's drifted zones stay as they were last
     read, so their alerts keep firing.
   - **`NbpdnsNoCompleteRefresh`** uses `3 * 300`, three times the default
     `drift.interval` of 5 minutes, in seconds. Change it with the interval.

2. Check the rules:

   ```shell
   promtool check rules nbpdns-rules.yml
   ```

   ```text
   Checking nbpdns-rules.yml
     SUCCESS: 5 rules found
   ```

3. Load them from `prometheus.yml`, and reload Prometheus:

   ```yaml
   rule_files:
     - nbpdns-rules.yml
   ```

The rules appear at **Alerts** in Prometheus's web UI. Route them to people
with Alertmanager, as for any other alert.

## See also

- The [metrics reference](../reference/metrics.md) lists every metric, with
  its labels.
- [How nbpdns runs as a service](../explanation/how-nbpdns-runs-as-a-service.md)
  explains why a group that can't be read keeps its last report, and why
  the health checks don't depend on NetBox.
