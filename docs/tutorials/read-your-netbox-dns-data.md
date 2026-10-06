---
title: Read your NetBox DNS data with nbpdns
weight: 10
---

# Read your NetBox DNS data with nbpdns

In this tutorial, you start the development lab, put a small zone into
NetBox, and read it back with nbpdns. Along the way, you check nbpdns's access
to NetBox, list the zones, and see a zone's records in the normalized form
that nbpdns compares with PowerDNS.

It takes about 15 minutes, most of it the lab's first start.

## Before you start

You need the repository and its
[development prerequisites](../../README.md#development), which include Docker.
Run every command from the repository's root.

## Start the lab and build nbpdns

1. Start the lab, which runs NetBox 4.7 with the NetBox DNS plugin:

   ```shell
   make lab-up
   ```

   The first start takes several minutes, while NetBox sets up its database.
   [Run the development lab](../how-to/run-the-development-lab.md) describes
   the lab in full.

2. Build nbpdns:

   ```shell
   make build
   ```

   The binary is `bin/nbpdns`.

## Put a zone into NetBox

NetBox is nbpdns's source of truth, so the DNS data starts there. You create
it through NetBox's REST API, as the lab's superuser, `admin`.

1. Set two shell variables: NetBox 4.7's URL, and the API token of the
   superuser.

   ```shell
   NETBOX=http://localhost:8047
   ADMIN_TOKEN=nbt_nbpdnslabadm.nbpdnsLabAdminTokenNotForProduction00000
   ```

2. Create a name server, and then the zone `tutorial.example`, which uses it:

   ```shell
   curl -X POST "$NETBOX/api/plugins/netbox-dns/nameservers/" \
     -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
     -d '{"name": "ns1.tutorial.example"}'
   curl -X POST "$NETBOX/api/plugins/netbox-dns/zones/" \
     -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
     -d '{"name": "tutorial.example",
          "nameservers": [{"name": "ns1.tutorial.example"}],
          "soa_mname": {"name": "ns1.tutorial.example"},
          "soa_rname": "hostmaster.tutorial.example"}'
   ```

   NetBox answers each request with the object it created, as JSON. The zone
   goes into the default view, `_default_`.

3. Add some records to the zone:

   ```shell
   curl -X POST "$NETBOX/api/plugins/netbox-dns/records/" \
     -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
     -d '[{"zone": {"name": "tutorial.example"}, "name": "ns1", "type": "A", "value": "192.0.2.53"},
          {"zone": {"name": "tutorial.example"}, "name": "www", "type": "A", "value": "192.0.2.10", "ttl": 300},
          {"zone": {"name": "tutorial.example"}, "name": "www", "type": "A", "value": "192.0.2.11"},
          {"zone": {"name": "tutorial.example"}, "name": "@", "type": "MX", "value": "10 mail"},
          {"zone": {"name": "tutorial.example"}, "name": "mail", "type": "A", "value": "192.0.2.25"},
          {"zone": {"name": "tutorial.example"}, "name": "@", "type": "TXT", "value": "v=spf1 mx -all"},
          {"zone": {"name": "tutorial.example"}, "name": "old", "type": "CNAME", "value": "www", "status": "inactive"}]'
   ```

   Notice three of them. The second `www` record has no TTL of its own. The
   `MX` record names its mail server relative to the zone, as `mail`. And the
   `old` record is inactive.

## Point nbpdns at NetBox

1. Create a config file, `nbpdns.yaml`, that names NetBox and asks for logs
   that are easy to read in a terminal:

   ```yaml
   log:
     format: text
   netbox:
     url: http://localhost:8047
   ```

2. Give nbpdns the token through the environment, which keeps it out of the
   file:

   ```shell
   export NBPDNS_NETBOX_TOKEN=$ADMIN_TOKEN
   ```

   > [!NOTE]
   > This tutorial reads with the token of the lab's superuser, to keep it
   > short.
   > Anywhere else, give nbpdns a token that can only read DNS data: see
   > [Give nbpdns read-only access to NetBox](../how-to/give-nbpdns-read-only-access-to-netbox.md).

3. See the settings nbpdns has now:

   ```shell
   bin/nbpdns config show --config nbpdns.yaml
   ```

   ```text
   KEY                 VALUE                  SOURCE
   log.format          text                   file nbpdns.yaml
   log.level           info                   default
   netbox.ca_file                             default
   netbox.concurrency  4                      default
   netbox.page_size    500                    default
   netbox.timeout      30s                    default
   netbox.token        [redacted]             env NBPDNS_NETBOX_TOKEN
   netbox.url          http://localhost:8047  file nbpdns.yaml
   ```

   Each setting shows where its value came from. The token shows as
   `[redacted]`: nbpdns never prints or logs it.

## Check nbpdns's access

Run the check:

```shell
bin/nbpdns netbox check --config nbpdns.yaml
```

```text
time=2026-10-06T01:13:49.886+10:00 level=WARN msg="the NetBox URL uses http://, so the token crosses the network unencrypted; use https:// if NetBox offers it" url=http://localhost:8047/ trace_id=367a… span_id=d510… request_id=VNYW…
CHECK                  RESULT   DETAIL
connection             warning  http://, so the token crosses the network unencrypted
netbox                 ok       NetBox 4.7.1
plugin                 ok       netbox_dns 1.7.2
token                  ok       a v2 token, accepted
netbox_dns.view        ok       can view 1
netbox_dns.zone        ok       can view 1
netbox_dns.nameserver  ok       can view 1
netbox_dns.record      ok       can view 9
```

The first line is a log line, on standard error. The lab's NetBox has no TLS,
so nbpdns warns that the token crosses the network unencrypted. The table, on
standard output, shows that NetBox runs a supported release with the DNS
plugin, accepted the token, and lets the token's user view each kind of
object that nbpdns reads. A warning doesn't fail the check; a failed check
would make nbpdns exit with status 1.

The rest of this tutorial leaves out the warning.

## List the zones

```shell
bin/nbpdns netbox zones --config nbpdns.yaml
```

```text
VIEW       ZONE               STATUS  SERIAL      DEFAULT TTL  NAMESERVERS
_default_  tutorial.example.  active  1791213230  86400        ns1.tutorial.example.
```

Your serial differs: NetBox sets it from the time of the last change. If you
used the lab before, you see other zones too.

nbpdns shows names the way it compares them: lowercase, and absolute, with a
final dot.

## Read the zone's records

1. List the records:

   ```shell
   bin/nbpdns netbox records --config nbpdns.yaml --zone tutorial.example
   ```

   ```text
   NAME                    TTL    TYPE   VALUE                                                                                  STATUS    MANAGED
   tutorial.example.       86400  SOA    ns1.tutorial.example. hostmaster.tutorial.example. 1791213230 43200 7200 2419200 3600  active    yes
   tutorial.example.       86400  NS     ns1.tutorial.example.                                                                  active    yes
   tutorial.example.       86400  MX     10 mail.tutorial.example.                                                              active    no
   tutorial.example.       86400  TXT    "v=spf1 mx -all"                                                                       active    no
   mail.tutorial.example.  86400  A      192.0.2.25                                                                             active    no
   ns1.tutorial.example.   86400  A      192.0.2.53                                                                             active    no
   old.tutorial.example.   86400  CNAME  www.tutorial.example.                                                                  inactive  no
   www.tutorial.example.   300    A      192.0.2.10                                                                             active    no
   www.tutorial.example.   300    A      192.0.2.11                                                                             active    no
   ```

   This is the zone in nbpdns's normalized form:

   - Records are grouped into **RRsets**: every record with one name and type,
     such as the two `www` A records. An RRset has one TTL. The second `www`
     record had no TTL of its own, so the DNS plugin gave it the first's, 300.
   - The `MX` record's `mail` is now absolute, `mail.tutorial.example.`, and
     so is the `CNAME` record's `www`.
   - The `TXT` value is quoted, as it would be in a zone file.
   - The DNS plugin made the `SOA` and `NS` records itself, from the zone's
     settings, so they're **managed**.
   - The inactive `old` record is listed with its status. It's in NetBox, but
     it isn't meant to be published.

2. Ask for the same records as JSON, which scripts can read:

   ```shell
   bin/nbpdns netbox records --config nbpdns.yaml --zone tutorial.example --output json
   ```

   The output is the zone, with its RRsets under `rrsets`, and a `problems`
   list:

   ```json
   {
     "name": "tutorial.example.",
     "view": "_default_",
     "status": "active",
     "active": true,
     "default_ttl": 86400,
     "soa_serial": 1791213230,
     "nameservers": [
       "ns1.tutorial.example."
     ],
     "rrsets": [
       {
         "name": "tutorial.example.",
         "type": "SOA",
         "ttl": 86400,
         "records": [
           {
             "value": "ns1.tutorial.example. hostmaster.tutorial.example. 1791213230 43200 7200 2419200 3600",
             "ttl": 86400,
             "status": "active",
             "active": true,
             "managed": true
           }
         ]
       }
     ],
     "problems": []
   }
   ```

   This excerpt shows only the first RRset. The `problems` list is empty
   here. It lists anything in NetBox's data that nbpdns had to work around,
   such as records in one RRset with different TTLs, which is possible
   only where the DNS plugin's checks are turned off.

## Clean up

Remove the lab, with its data, and the config file:

```shell
make lab-down
rm nbpdns.yaml
```

## Next steps

- [How nbpdns reads NetBox](../explanation/how-nbpdns-reads-netbox.md)
  explains the normalized form, and how nbpdns pages through NetBox's API.
- [Give nbpdns read-only access to NetBox](../how-to/give-nbpdns-read-only-access-to-netbox.md)
  sets up the token nbpdns should use.
- [Configure nbpdns](../how-to/configure-nbpdns.md) covers the config file,
  environment variables, flags, and secret files.
- The [command-line reference](../reference/command-line.md) lists every
  command and flag.
