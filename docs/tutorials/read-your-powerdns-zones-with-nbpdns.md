---
title: Read your PowerDNS zones with nbpdns
weight: 20
---

# Read your PowerDNS zones with nbpdns

In this tutorial, you put a small zone into the development lab's PowerDNS
server, describe the lab's server groups to nbpdns, and read the zone back.
Along the way, you check nbpdns's access to each group's primary, list the
zones, and see a zone's records in the normalized form that nbpdns compares
with NetBox.

It takes about 10 minutes, most of it the lab's first start.

## Before you start

You need the repository and its
[development prerequisites](../../README.md#development), which include Docker.
Run every command from the repository's root. If you've done
[Read your NetBox DNS data with nbpdns](read-your-netbox-dns-data.md), the lab
may already be running.

## Start the lab and build nbpdns

1. Start the lab:

   ```shell
   make lab-up
   ```

   Besides NetBox, the lab runs a PowerDNS 5.1 server, the primary of the
   server group `lab-a`.
   [Run the development lab](../how-to/run-the-development-lab.md) describes
   the lab in full.

2. Build nbpdns:

   ```shell
   make build
   ```

   The binary is `bin/nbpdns`.

## Put a zone into PowerDNS

Later, nbpdns writes zones to PowerDNS from NetBox's data. For now, you create
one yourself, through PowerDNS's API, on lab-a's primary.

1. Set two shell variables: the address of the API of lab-a's primary, and
   the lab's API key.

   ```shell
   PDNS=http://localhost:8151/api/v1/servers/localhost
   PDNS_KEY=nbpdns-lab-powerdns-api-key-not-for-production
   ```

2. Create the zone `tutorial.example`, with a few records:

   ```shell
   curl -X POST "$PDNS/zones" \
     -H "X-API-Key: $PDNS_KEY" -H "Content-Type: application/json" \
     -d '{"name": "tutorial.example.", "kind": "Native", "nameservers": [],
          "rrsets": [
            {"name": "tutorial.example.", "type": "SOA", "ttl": 3600,
             "records": [{"content": "ns1.tutorial.example. hostmaster.tutorial.example. 1 10800 3600 604800 3600", "disabled": false}]},
            {"name": "tutorial.example.", "type": "NS", "ttl": 3600,
             "records": [{"content": "ns1.tutorial.example.", "disabled": false}]},
            {"name": "www.tutorial.example.", "type": "A", "ttl": 300,
             "records": [{"content": "192.0.2.10", "disabled": false}, {"content": "192.0.2.11", "disabled": true}]},
            {"name": "tutorial.example.", "type": "MX", "ttl": 3600,
             "records": [{"content": "10 Mail.Tutorial.Example.", "disabled": false}]},
            {"name": "_443._tcp.www.tutorial.example.", "type": "TLSA", "ttl": 3600,
             "records": [{"content": "3 1 1 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "disabled": false}]}]}'
   ```

   PowerDNS answers with the zone it created, as JSON. Notice three things.
   PowerDNS groups records into **RRsets**, each with one TTL: the two `www`
   addresses are one RRset. The second `www` address has `"disabled": true`,
   so PowerDNS keeps it but doesn't serve it. And the `MX` record names its
   mail server in mixed case.

## Describe the server groups to nbpdns

nbpdns reads PowerDNS through **server groups**: each is a primary, the server
nbpdns talks to, and the NetBox views whose zones it serves.

1. Put the API key in a file, which keeps it out of the config file:

   ```shell
   printf '%s\n' "$PDNS_KEY" > pdns-lab.key
   ```

2. Create a config file, `nbpdns.yaml`, that declares the lab's group and
   asks for logs that are easy to read in a terminal:

   ```yaml
   log:
     format: text
   powerdns:
     groups:
       - name: lab-a
         views: [_default_]
         primary:
           url: http://localhost:8151
           api_key_file: pdns-lab.key
   ```

   The group serves NetBox's default view, `_default_`. Server groups can only
   be declared in the config file, and there can be as many as you have sites
   or clusters.

3. See the settings nbpdns has for lab-a:

   ```shell
   bin/nbpdns config show --config nbpdns.yaml | grep -E 'KEY|lab-a'
   ```

   ```text
   KEY                                         VALUE                  SOURCE
   powerdns.groups.lab-a.name                  lab-a                  file nbpdns.yaml
   powerdns.groups.lab-a.primary.api_key       [redacted]             file nbpdns.yaml
   powerdns.groups.lab-a.primary.api_key_file  pdns-lab.key           file nbpdns.yaml
   powerdns.groups.lab-a.primary.ca_file                              default
   powerdns.groups.lab-a.primary.cert_file                            default
   powerdns.groups.lab-a.primary.key_file                             default
   powerdns.groups.lab-a.primary.server_id     localhost              default
   powerdns.groups.lab-a.primary.url           http://localhost:8151  file nbpdns.yaml
   powerdns.groups.lab-a.views                 _default_              file nbpdns.yaml
   ```

   The API key shows as `[redacted]`: nbpdns never prints or logs it.

## Check nbpdns's access

Run the check:

```shell
bin/nbpdns powerdns check --config nbpdns.yaml
```

```text
time=2026-10-06T15:15:33.994+10:00 level=WARN msg="a PowerDNS API URL uses http://, so its API key, which can change every zone, crosses the network unencrypted; put the API behind TLS" group=lab-a url=http://localhost:8151/ trace_id=7895… span_id=8404… request_id=6CXQ…
GROUP  CHECK       RESULT   DETAIL
lab-a  connection  warning  http://, so the API key, which can change every zone, crosses the network unencrypted
lab-a  server      ok       PowerDNS Authoritative Server 5.1.4
lab-a  key         ok       accepted
lab-a  zones       ok       can read 1
```

The first line is a log line, on standard error. The lab server's API has no
TLS, so nbpdns warns that a key that can change every zone crosses the
network unencrypted. The table, on standard output, has rows for each group:
its primary is a PowerDNS Authoritative Server of a supported release,
accepted the key, and listed its zones. A warning doesn't fail the check.

> [!WARNING]
> Outside the lab, put each PowerDNS API behind TLS, as
> [Put the PowerDNS API behind a TLS proxy](../how-to/put-the-powerdns-api-behind-a-tls-proxy.md)
> explains.

The rest of this tutorial leaves out the warning.

## List the zones

```shell
bin/nbpdns powerdns zones --config nbpdns.yaml
```

```text
GROUP  ZONE               KIND    SERIAL      CATALOG
lab-a  tutorial.example.  Native  2026100601
```

Your serial differs. You sent `1`, but PowerDNS sets the serial itself, from
the date, whenever its API changes a zone. With several groups declared, add
`--group` to list only one group's zones.

## Read the zone's records

1. List the records:

   ```shell
   bin/nbpdns powerdns records --config nbpdns.yaml --group lab-a --zone tutorial.example
   ```

   ```text
   NAME                             TTL   TYPE  VALUE                                                                                 STATUS
   tutorial.example.                3600  SOA   ns1.tutorial.example. hostmaster.tutorial.example. 2026100601 10800 3600 604800 3600  active
   tutorial.example.                3600  NS    ns1.tutorial.example.                                                                 active
   tutorial.example.                3600  MX    10 mail.tutorial.example.                                                             active
   www.tutorial.example.            300   A     192.0.2.10                                                                            active
   www.tutorial.example.            300   A     192.0.2.11                                                                            disabled
   _443._tcp.www.tutorial.example.  3600  TLSA  3 1 1 0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF                active
   ```

   This is the zone in nbpdns's normalized form, the same form it reads
   NetBox's data into:

   - The `MX` record's mail server is in lowercase, `mail.tutorial.example.`,
     since DNS names don't depend on case.
   - The `TLSA` record's hex digest is in uppercase, for the same reason.
   - The second `www` address is listed with its status, `disabled`.
     PowerDNS keeps it, but doesn't serve it.

   `--group` says which group's primary to read. With only one group
   declared, you can leave it out.

2. Ask for the same records as JSON, which scripts can read:

   ```shell
   bin/nbpdns powerdns records --config nbpdns.yaml --group lab-a --zone tutorial.example --output json
   ```

   The output is the zone, with the group's name, its RRsets under `rrsets`,
   and a `problems` list. This excerpt shows only the `www` RRset:

   ```json
   {
     "group": "lab-a",
     "name": "tutorial.example.",
     "active": true,
     "soa_serial": 2026100601,
     "nameservers": [
       "ns1.tutorial.example."
     ],
     "rrsets": [
       {
         "name": "www.tutorial.example.",
         "type": "A",
         "ttl": 300,
         "records": [
           {
             "value": "192.0.2.10",
             "ttl": 300,
             "status": "active",
             "active": true,
             "managed": false
           },
           {
             "value": "192.0.2.11",
             "ttl": 300,
             "status": "disabled",
             "active": false,
             "managed": false
           }
         ]
       }
     ],
     "problems": []
   }
   ```

   The `problems` list is empty here. It lists anything in PowerDNS's data that
   nbpdns had to work around, such as a value that doesn't parse as its type.

## Clean up

Remove the lab, with its data, and the files you made:

```shell
make lab-down
rm nbpdns.yaml pdns-lab.key
```

## Next steps

- [How nbpdns reads PowerDNS](../explanation/how-nbpdns-reads-powerdns.md)
  explains server groups, how zones are assigned to them, and why the API key
  needs care.
- [Connect nbpdns to PowerDNS](../how-to/connect-nbpdns-to-powerdns.md) sets up
  server groups outside the lab.
- The [configuration reference](../reference/configuration.md#powerdnsgroups)
  lists every field of a server group.
