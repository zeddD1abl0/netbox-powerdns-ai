---
title: Connect nbpdns to PowerDNS
weight: 25
---

# Connect nbpdns to PowerDNS

nbpdns reads each PowerDNS server group through its primary's HTTP API. This
guide gives each primary an API key, declares the server groups in nbpdns's
config file, and checks that nbpdns can read every primary.

## Before you start

You need:

- for each server group, a primary running a
  [supported release](../reference/supported-versions.md#powerdns) of the
  PowerDNS Authoritative Server, with a backend its API can write to, such as
  `gpgsql`, `gmysql`, or `lmdb`;
- nbpdns, with a config file (see [Configure nbpdns](configure-nbpdns.md));
- the names of the NetBox views whose zones each group serves.

> [!WARNING]
> A PowerDNS API key can't be limited: it can change every zone, record, TSIG
> key, and DNSSEC key on its server. Keep the API off the network in clear
> text with [a TLS proxy](put-the-powerdns-api-behind-a-tls-proxy.md), and
> keep the key where only nbpdns can read it.

## Give each primary an API key

On each primary:

1. Make a long random key:

   ```shell
   openssl rand -base64 32
   ```

2. Hash the key, so that `pdns.conf` doesn't hold it in clear text. Paste the
   key when `pdnsutil` asks for it:

   ```shell
   pdnsutil hash-password
   ```

   The hash starts with `$scrypt$`.

3. Turn on the API in `pdns.conf`, with the hash as the key:

   ```ini
   api=yes
   api-key=$scrypt$ln=10,p=1,r=8$...
   webserver=yes
   webserver-address=127.0.0.1
   webserver-port=8081
   webserver-allow-from=127.0.0.1,::1
   ```

   These settings keep the API on the primary's own address, for a TLS proxy
   to reach. To reach the API without a proxy, set `webserver-address` and
   `webserver-allow-from` to addresses that nbpdns can use instead.

4. Restart PowerDNS, and check that the API answers with the key:

   ```shell
   curl -H "X-API-Key: <the key>" http://127.0.0.1:8081/api/v1/servers/localhost
   ```

## Declare the server groups

1. On nbpdns's host, put each primary's key in a file that only nbpdns's user
   can read:

   ```shell
   install -m 600 /dev/null /etc/nbpdns/pdns-site-a.key
   printf '%s\n' '<the key>' > /etc/nbpdns/pdns-site-a.key
   ```

2. Declare each group under `powerdns.groups` in the config file, with its
   name, the NetBox views it serves, and its primary:

   ```yaml
   powerdns:
     groups:
       - name: site-a
         views: [_default_, internal]
         primary:
           url: https://pdns-a.example.com:8443
           api_key_file: /etc/nbpdns/pdns-site-a.key
       - name: site-b
         views: [_default_]
         primary:
           url: https://pdns-b.example.com:8443
           api_key_file: /etc/nbpdns/pdns-site-b.key
   ```

   - A view may be served by more than one group, as `_default_` is here.
   - If the primary's TLS certificate comes from a private CA, add `ca_file`.
     If its proxy asks for a client certificate, add `cert_file` and
     `key_file`.
   - `server_id` is `localhost` unless the primary says otherwise.

   The [configuration reference](../reference/configuration.md#powerdnsgroups)
   lists every field.

## Check the connection

1. Check every group's primary:

   ```shell
   nbpdns powerdns check --config /etc/nbpdns/nbpdns.yaml
   ```

   Each group should pass the `connection`, `server`, `key`, and `zones`
   checks. A failed check names its cause: an unreachable address, a rejected
   key, a server that isn't authoritative, or an unsupported release.

2. List the zones on each primary:

   ```shell
   nbpdns powerdns zones --config /etc/nbpdns/nbpdns.yaml
   ```

3. List the NetBox zones that a group serves, through its views:

   ```shell
   nbpdns netbox zones --config /etc/nbpdns/nbpdns.yaml --group site-a
   ```

   One PowerDNS server holds one zone of each name. If a zone name is in two
   of a group's views, nbpdns logs a warning naming them: fix the group's
   views, or the zones in NetBox.
