---
title: Export traces to an OpenTelemetry collector
weight: 45
---

# Export traces to an OpenTelemetry collector

nbpdns records a trace of each command it runs, and of each refresh of
`nbpdns serve`, with a span for every request it makes to NetBox and to
PowerDNS. This guide sends those traces to an OpenTelemetry collector, over
OTLP by HTTP or by gRPC.

## Before you start

You need:

- an OpenTelemetry Collector, or any backend that receives OTLP, such as
  Jaeger, Grafana Tempo, or a vendor's endpoint, usually on port 4318 for
  HTTP and 4317 for gRPC;
- if it needs one, the token or header that it authenticates exporters
  with;
- if its certificate comes from a private CA, the CA certificate, in a PEM
  file.

## Point nbpdns at the collector

1. Set the endpoint and the protocol in nbpdns's config file:

   ```yaml
   otlp:
     endpoint: https://otel.example.com:4318
     protocol: http/protobuf
   ```

   For gRPC, set `protocol: grpc`, and give the collector's gRPC port,
   usually 4317. For `http/protobuf`, nbpdns adds `/v1/traces` to the
   endpoint's path, so give the collector's base URL.

2. If the collector's certificate comes from a private CA, add the CA
   certificate:

   ```yaml
   otlp:
     ca_file: /etc/nbpdns/otel-ca.pem
   ```

3. If the collector wants a header, such as a token, put it in a file that
   only nbpdns's user can read, as `name=value`:

   ```shell
   install -m 600 /dev/null /etc/nbpdns/otlp-headers
   printf '%s\n' 'Authorization=Bearer <the token>' > /etc/nbpdns/otlp-headers
   ```

   Then name the file in the config file:

   ```yaml
   otlp:
     headers_file: /etc/nbpdns/otlp-headers
   ```

   Separate more headers with commas, and percent-encode a comma in a
   value as `%2C`. Over gRPC, the headers are sent as metadata. nbpdns
   never prints or logs them.

> [!WARNING]
> An `http://` endpoint sends the spans and the headers unencrypted, and
> nbpdns logs a warning each time it exports that way.
> Use `https://` wherever the collector offers it.

## Check the export

1. Run a short command:

   ```shell
   nbpdns netbox check --config /etc/nbpdns/nbpdns.yaml
   ```

   nbpdns sends its last spans as the command ends, waiting up to
   `otlp.timeout`, 10 seconds by default.

2. In the collector's backend, look for a trace from the service `nbpdns`
   named `nbpdns netbox check`, with a child span for each request to
   NetBox.

3. With `nbpdns serve` running, look for a trace named `drift refresh` for
   each refresh. Each request's span carries the URL and the status, and
   the request itself carries the W3C `traceparent` header, so a server
   that records it joins the trace.

If spans don't arrive, look in nbpdns's log for `couldn't export spans to
the OpenTelemetry collector`, which gives the collector's answer or the
connection error.

## See also

- The [configuration reference](../reference/configuration.md#otlpendpoint)
  lists every `otlp` key.
- [How nbpdns runs as a service](../explanation/how-nbpdns-runs-as-a-service.md)
  explains why each refresh is a trace of its own.
