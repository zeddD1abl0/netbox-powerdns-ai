---
title: Put the PowerDNS API behind a TLS proxy
weight: 26
---

# Put the PowerDNS API behind a TLS proxy

PowerDNS's API has no TLS of its own, and its key can change every zone on
the server. This guide puts the API behind a TLS reverse proxy on the same
host, so the key never crosses the network in clear text, and can also make
the proxy demand a client certificate from nbpdns. It's the reference setup
of [ADR-0024](../adr/0024-read-powerdns-through-its-api-from-server-groups-i.md),
and uses nginx; any reverse proxy that terminates TLS works the same way.

## Before you start

You need:

- the PowerDNS primary's API on its own address, `127.0.0.1:8081`, with a
  hashed key: see [Connect nbpdns to PowerDNS](connect-nbpdns-to-powerdns.md);
- nginx on the same host;
- a TLS certificate and key for the host's name, such as
  `pdns-a.example.com`, from a CA that nbpdns trusts;
- to require a client certificate, a CA for client certificates, and a
  certificate and key it issued for nbpdns.

## Set up the proxy

1. Put the certificates in `/etc/nginx/tls/`: the host's certificate as
   `server.pem` and its key as `server-key.pem`, and, for client
   certificates, their CA as `client-ca.pem`.

2. Add a server to nginx, such as in `/etc/nginx/conf.d/pdns-api.conf`:

   ```nginx
   server {
       listen 8443 ssl;
       server_name pdns-a.example.com;

       ssl_certificate     /etc/nginx/tls/server.pem;
       ssl_certificate_key /etc/nginx/tls/server-key.pem;
       ssl_protocols       TLSv1.2 TLSv1.3;

       # Optional: only clients with a certificate from this CA get through.
       ssl_client_certificate /etc/nginx/tls/client-ca.pem;
       ssl_verify_client      on;

       location /api/ {
           proxy_pass http://127.0.0.1:8081;
       }
   }
   ```

   The proxy passes every method, so it also serves nbpdns once it writes
   zones. Leave out the two `ssl_client_*` lines if you don't use client
   certificates.

3. Check the configuration, and reload nginx:

   ```shell
   nginx -t && nginx -s reload
   ```

4. Allow port 8443 through the host's firewall from nbpdns's address only.

## Point nbpdns at the proxy

1. In nbpdns's config file, give the group's primary the proxy's `https://`
   URL, and the files nbpdns needs:

   ```yaml
   powerdns:
     groups:
       - name: site-a
         views: [_default_]
         primary:
           url: https://pdns-a.example.com:8443
           api_key_file: /etc/nbpdns/pdns-site-a.key
           ca_file: /etc/nbpdns/pdns-ca.pem
           cert_file: /etc/nbpdns/client.pem
           key_file: /etc/nbpdns/client-key.pem
   ```

   - `ca_file` is needed only if the proxy's certificate comes from a private
     CA.
   - `cert_file` and `key_file` are the client certificate and its key, if
     the proxy asks for one.

2. Check the group:

   ```shell
   nbpdns powerdns check --config /etc/nbpdns/nbpdns.yaml --group site-a
   ```

   The `connection` check now passes with `https://`, and nbpdns no longer
   warns that the key crosses the network unencrypted.

## If the check fails

- **`x509: certificate signed by unknown authority`:** nbpdns doesn't trust
  the proxy's certificate. Set `ca_file` to the certificate of the CA that
  issued it.
- **`status 400: 400 No required SSL certificate was sent`:** the proxy wants
  a client certificate. Set `cert_file` and `key_file`.
- **`PowerDNS rejected the API key`:** the proxy works, and PowerDNS refused
  the key. Check the key file against the key you hashed.
