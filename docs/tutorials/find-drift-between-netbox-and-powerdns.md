---
title: Find drift between NetBox and PowerDNS
weight: 30
---

# Find drift between NetBox and PowerDNS

In this tutorial, you put a zone into the development lab's NetBox, put a
slightly different copy of it on the lab's PowerDNS server, and ask nbpdns
what differs. Along the way, you see a zone that's missing on PowerDNS, each
kind of difference between two copies of a zone, a zone that nbpdns doesn't
manage, and the exit statuses that scripts can act on.

**Drift** is any difference between what NetBox says a zone holds and what
PowerDNS serves. NetBox is the source of truth, so drift means PowerDNS is
wrong.

It takes about 15 minutes, most of it the lab's first start.

## Before you start

You need the repository and its
[development prerequisites](../../README.md#development), which include Docker.
Run every command from the repository's root.

The tutorials
[Read your NetBox DNS data with nbpdns](read-your-netbox-dns-data.md) and
[Read your PowerDNS zones with nbpdns](read-your-powerdns-zones-with-nbpdns.md)
explain more of what you see here, but you don't need to do them first. If
you have, and the lab is still running, run `make lab-down` to start again
from an empty lab.

## Start the lab and build nbpdns

1. Start the lab:

   ```shell
   make lab-up
   ```

   The lab runs NetBox 4.7 with the NetBox DNS plugin, and a PowerDNS 5.1
   server, the primary of the server group `lab-a`.
   [Run the development lab](../how-to/run-the-development-lab.md) describes
   the lab in full.

2. Build nbpdns:

   ```shell
   make build
   ```

   The binary is `bin/nbpdns`.

## Put a zone into NetBox

1. Set two shell variables: NetBox's URL, and the API token of the lab's
   superuser.

   ```shell
   NETBOX=http://localhost:8047
   ADMIN_TOKEN=nbt_nbpdnslabadm.nbpdnsLabAdminTokenNotForProduction00000
   ```

2. Create a name server, and then the zone `drift.example`:

   ```shell
   curl -X POST "$NETBOX/api/plugins/netbox-dns/nameservers/" \
     -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
     -d '{"name": "ns1.drift.example"}'
   curl -X POST "$NETBOX/api/plugins/netbox-dns/zones/" \
     -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
     -d '{"name": "drift.example", "default_ttl": 3600,
          "nameservers": [{"name": "ns1.drift.example"}],
          "soa_mname": {"name": "ns1.drift.example"},
          "soa_rname": "hostmaster.drift.example",
          "soa_ttl": 3600, "soa_refresh": 10800, "soa_retry": 3600,
          "soa_expire": 604800, "soa_minimum": 3600}'
   ```

   The zone sets its SOA's timers, rather than taking the plugin's defaults,
   so that they match the copy you put on PowerDNS later.

3. Add four records:

   ```shell
   curl -X POST "$NETBOX/api/plugins/netbox-dns/records/" \
     -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
     -d '[{"zone": {"name": "drift.example"}, "name": "www", "type": "A", "value": "192.0.2.10"},
          {"zone": {"name": "drift.example"}, "name": "api", "type": "A", "value": "192.0.2.30"},
          {"zone": {"name": "drift.example"}, "name": "mail", "type": "A", "value": "192.0.2.25"},
          {"zone": {"name": "drift.example"}, "name": "@", "type": "MX", "value": "10 mail"}]'
   ```

   None of them has a TTL of its own, so each takes the zone's default,
   3600 seconds.

## Describe NetBox and lab-a to nbpdns

1. Put lab-a's API key in a file, which keeps it out of the config file:

   ```shell
   printf '%s\n' nbpdns-lab-powerdns-api-key-not-for-production > pdns-lab.key
   ```

2. Create a config file, `nbpdns.yaml`, that names NetBox, declares the
   server group `lab-a`, and asks for logs that are easy to read in a
   terminal:

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
   ```

   The group serves the zones in NetBox's default view, `_default_`, where
   `drift.example` is.

3. Give nbpdns the NetBox token through the environment:

   ```shell
   export NBPDNS_NETBOX_TOKEN=$ADMIN_TOKEN
   ```

   > [!NOTE]
   > This tutorial reads with the token of the lab's superuser, to keep it
   > short. Anywhere else, give nbpdns a token that can only read DNS data:
   > see [Give nbpdns read-only access to NetBox](../how-to/give-nbpdns-read-only-access-to-netbox.md).

## Find a zone that's missing

1. Ask nbpdns for drift:

   ```shell
   bin/nbpdns drift --config nbpdns.yaml
   ```

   ```text
   time=2026-10-06T22:25:24.101+10:00 level=WARN msg="the NetBox URL uses http://, so the token crosses the network unencrypted; use https:// if NetBox offers it" url=http://localhost:8047/ trace_id=e219… span_id=8c5e… request_id=ANS3…
   time=2026-10-06T22:25:24.110+10:00 level=WARN msg="a PowerDNS API URL uses http://, so its API key, which can change every zone, crosses the network unencrypted; put the API behind TLS" group=lab-a url=http://localhost:8151/ trace_id=e219… span_id=8c5e… request_id=ANS3…
   GROUP  STATUS  IN SYNC  DRIFT  MISSING  INACTIVE  IGNORED  UNMANAGED
   lab-a  ok      0        0      1        0         0        0

   Drift:
   GROUP  ZONE            POLICY  CHANGE                       NAME  TYPE  NETBOX  POWERDNS
   lab-a  drift.example.  report  zone missing on the primary
   nbpdns: 1 zone drifted
   ```

   The first two lines are logs, on standard error. The lab has no TLS, so
   nbpdns warns that the token and the API key cross the network
   unencrypted. The rest of this tutorial leaves them out.

   The report starts with a summary line for each server group: how many of
   the zones that NetBox assigns to it are in each state. `drift.example` is
   active in NetBox, in a view that lab-a serves, so lab-a's primary should
   serve it, and it doesn't: it's `MISSING`. The drift section says so, with
   the zone's drift policy, `report`, which every zone has unless you set
   another.

2. See the exit status:

   ```shell
   echo $?
   ```

   ```text
   3
   ```

   nbpdns exits with 3 when it compared everything and found drift, 0 when it
   found none, and 1 when it couldn't read NetBox or a primary. A script or a
   CI job can act on the status alone.

## Put a different copy on PowerDNS

Later, nbpdns corrects drift itself. For now, you put the zone on lab-a's
primary by hand, through PowerDNS's API, with a few differences from NetBox.

1. Set two shell variables: the address of the API of lab-a's primary, and
   its API key.

   ```shell
   PDNS=http://localhost:8151/api/v1/servers/localhost
   PDNS_KEY=$(cat pdns-lab.key)
   ```

2. Create the zone:

   ```shell
   curl -X POST "$PDNS/zones" \
     -H "X-API-Key: $PDNS_KEY" -H "Content-Type: application/json" \
     -d '{"name": "drift.example.", "kind": "Native", "nameservers": [],
          "rrsets": [
            {"name": "drift.example.", "type": "SOA", "ttl": 3600,
             "records": [{"content": "ns1.drift.example. hostmaster.drift.example. 1 10800 3600 604800 3600", "disabled": false}]},
            {"name": "drift.example.", "type": "NS", "ttl": 3600,
             "records": [{"content": "ns1.drift.example.", "disabled": false}]},
            {"name": "www.drift.example.", "type": "A", "ttl": 3600,
             "records": [{"content": "192.0.2.99", "disabled": false}]},
            {"name": "mail.drift.example.", "type": "A", "ttl": 300,
             "records": [{"content": "192.0.2.25", "disabled": false}]},
            {"name": "drift.example.", "type": "MX", "ttl": 3600,
             "records": [{"content": "10 mail.drift.example.", "disabled": false}]},
            {"name": "old.drift.example.", "type": "TXT", "ttl": 3600,
             "records": [{"content": "\"left over\"", "disabled": false}]}]}'
   ```

   Compared with NetBox, `www` has another address, `mail` has a TTL of 300
   seconds, `api` isn't there, and there's an `old` TXT record that NetBox
   doesn't have.

3. Ask nbpdns for drift again:

   ```shell
   bin/nbpdns drift --config nbpdns.yaml
   ```

   ```text
   GROUP  STATUS  IN SYNC  DRIFT  MISSING  INACTIVE  IGNORED  UNMANAGED
   lab-a  ok      0        1      0        0         0        0

   Drift:
   GROUP  ZONE            POLICY  CHANGE   NAME                 TYPE  NETBOX           POWERDNS
   lab-a  drift.example.  report  missing  api.drift.example.   A     3600 192.0.2.30  -
   lab-a  drift.example.  report  changed  mail.drift.example.  A     3600 192.0.2.25  300 192.0.2.25
   lab-a  drift.example.  report  extra    old.drift.example.   TXT   -                3600 "left over"
   lab-a  drift.example.  report  changed  www.drift.example.   A     3600 192.0.2.10  3600 192.0.2.99
   nbpdns: 1 zone drifted
   ```

   The zone is now in the `DRIFT` column, and each row of the drift section
   is one **RRset**: the records of one name and type. `NETBOX` and
   `POWERDNS` give each side's TTL and values, or `-` where that side
   doesn't have the RRset:

   - `missing`: NetBox has the RRset, and the primary doesn't serve it.
   - `extra`: the primary serves the RRset, and NetBox doesn't have it.
   - `changed`: both have it. Its values differ, or its TTL does.

   Notice what isn't listed. The two SOA records have different serials:
   PowerDNS set its own, from the date, when you created the zone. nbpdns
   compares every other field of the SOA, but not the serial. And the `MX`
   records match, although NetBox names the mail server as `mail`, and
   PowerDNS as `mail.drift.example.`: nbpdns reads both into one form before
   it compares them.

## See the report as JSON

1. Ask for the same report as JSON:

   ```shell
   bin/nbpdns drift --config nbpdns.yaml --output json
   ```

   This excerpt shows the zone, with only its first change:

   ```json
   {
     "zone": "drift.example.",
     "view": "_default_",
     "policy": "report",
     "state": "drift",
     "netbox_serial": 1791289524,
     "powerdns_serial": 2026100601,
     "changes": [
       {
         "name": "api.drift.example.",
         "type": "A",
         "kind": "missing",
         "netbox": {
           "ttl": 3600,
           "values": [
             "192.0.2.30"
           ]
         }
       }
     ]
   }
   ```

   The JSON has everything the table has, and more, such as each side's SOA
   serial. Your serials differ. Around the zones, it says whether the report is `complete` and
   whether it found `drift`, and gives each group's zones, counts, and
   `unmanaged` zones, and the `problems` nbpdns worked around in the data.

## Fix the drift

nbpdns doesn't write to PowerDNS yet, so fix the primary by hand.

1. Change the primary's copy to match NetBox:

   ```shell
   curl -X PATCH "$PDNS/zones/drift.example." \
     -H "X-API-Key: $PDNS_KEY" -H "Content-Type: application/json" \
     -d '{"rrsets": [
            {"name": "www.drift.example.", "type": "A", "ttl": 3600, "changetype": "REPLACE",
             "records": [{"content": "192.0.2.10", "disabled": false}]},
            {"name": "mail.drift.example.", "type": "A", "ttl": 3600, "changetype": "REPLACE",
             "records": [{"content": "192.0.2.25", "disabled": false}]},
            {"name": "api.drift.example.", "type": "A", "ttl": 3600, "changetype": "REPLACE",
             "records": [{"content": "192.0.2.30", "disabled": false}]},
            {"name": "old.drift.example.", "type": "TXT", "changetype": "DELETE"}]}'
   ```

   PowerDNS answers with no content.

2. Ask nbpdns for drift, and see the exit status:

   ```shell
   bin/nbpdns drift --config nbpdns.yaml; echo $?
   ```

   ```text
   GROUP  STATUS  IN SYNC  DRIFT  MISSING  INACTIVE  IGNORED  UNMANAGED
   lab-a  ok      1        0      0        0         0        0
   0
   ```

   The zone is in sync, there's no drift section, and the exit status is 0.

## See a zone that nbpdns doesn't manage

A primary can hold zones that NetBox doesn't know about, such as ones made
before nbpdns arrived.

1. Create a zone on lab-a's primary only:

   ```shell
   curl -X POST "$PDNS/zones" \
     -H "X-API-Key: $PDNS_KEY" -H "Content-Type: application/json" \
     -d '{"name": "legacy.example.", "kind": "Native", "nameservers": ["ns1.drift.example."]}'
   ```

2. Ask nbpdns for drift:

   ```shell
   bin/nbpdns drift --config nbpdns.yaml; echo $?
   ```

   ```text
   GROUP  STATUS  IN SYNC  DRIFT  MISSING  INACTIVE  IGNORED  UNMANAGED
   lab-a  ok      1        0      0        0         0        1

   Unmanaged zones, on a primary but not in its group's NetBox views:
   GROUP  ZONE
   lab-a  legacy.example.
   0
   ```

   `legacy.example.` is **unmanaged**: lab-a's primary serves it, but NetBox
   doesn't assign it to lab-a. nbpdns lists it, so that it doesn't go
   unseen, but it isn't drift, and the exit status is still 0.

3. Compare only one zone:

   ```shell
   bin/nbpdns drift --config nbpdns.yaml --zone drift.example
   ```

   ```text
   GROUP  STATUS  IN SYNC  DRIFT  MISSING  INACTIVE  IGNORED  UNMANAGED
   lab-a  ok      1        0      0        0         0        0
   ```

   With `--zone`, nbpdns reads only that zone from NetBox and from each
   primary. It doesn't read `legacy.example.` at all. With several server groups,
   `--group` compares only one of them.

## Clean up

Remove the lab, with its data, and the files you made:

```shell
make lab-down
rm nbpdns.yaml pdns-lab.key
```

## Next steps

- [How nbpdns finds drift](../explanation/how-nbpdns-finds-drift.md)
  explains what's compared, what isn't, and why.
- [Set a zone's drift policy](../how-to/set-a-zones-drift-policy.md) leaves
  a zone out of the report.
- The [command-line reference](../reference/command-line.md#nbpdns-drift)
  lists every flag of `nbpdns drift`, and every exit status.
