---
id: ITEM-0067
title: The API's middleware
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M06
requirements: [REQ-037, REQ-046]
depends_on: [ITEM-0066]
created: 2026-10-08
closed: 2026-10-08
---

# ITEM-0067: The API's middleware

## Goal

The behaviour every API request shares (ADR-0033): problem details
for every error under `/api`, unknown paths and methods included;
`X-Flow-ID` taken or made, returned, and used as the request ID; a span
per request that continues an incoming `traceparent`; a log line per
request; request metrics; and absolute links from `server.public_url`.

## Acceptance criteria

- [x] Every error under `/api`, from 400 to 500, unknown paths and methods included, is `application/problem+json`, validated against the spec.
- [x] A request's `X-Flow-ID` is returned, or a new one is made; it's the request ID in the logs; an invalid one is replaced.
- [x] An incoming `traceparent` is continued by the request's span.
- [x] `nbpdns_api_requests_total` and `nbpdns_api_request_duration_seconds` are declared, and their reference regenerated.
- [x] `server.public_url` is declared, and links are absolute with and without it.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M06's approved design.
- 2026-10-08: Done.
  - **`internal/api`'s handler** wraps the mux, for every request:
    - **Flow ID:** a request's `X-Flow-ID` is kept if it matches the
      spec's pattern, 1 to 128 of `[A-Za-z0-9._:+/=-]`, and is replaced
      otherwise. The response returns it, the request's header carries it
      to the handlers, and it's the request ID in the logs.
    - **Tracing:** a server span, named by the route, such as
      `GET /api/status`, continues an incoming W3C `traceparent`. It has
      `http.request.method`, `url.path`, `http.route` and
      `http.response.status_code`, and an error status from 500 up.
    - **Problems:** a request that matches no route is probed against the
      mux. An unknown path is a 404 problem. A known path with another
      method is a 405 problem, with the mux's `Allow`. Anything else,
      such as the mux's 307 to a cleaned path, passes through.
    - **A log line** for each request, at info: method, path, operation,
      status and duration, with the request and trace IDs.
    - **Metrics:** `nbpdns_api_requests_total{operation,code}` and
      `nbpdns_api_request_duration_seconds{operation}`. The operation is
      the route's `operationId`, mapped at startup from the embedded spec,
      so it's right even when parameter binding fails before the handler
      runs. It's `openapi` for the spec, and `unmatched` for anything
      else.
    - `Cache-Control: no-store`, which moved here from ITEM-0066's
      wrapper.
  - **Errors:**
    - a parameter the generated code can't bind is a 400 problem, with its
      error as the detail;
    - a handler's error is a 500 problem whose detail points to the log,
      under the flow ID, and the error itself is only logged.
  - **`server.public_url`**, a bootstrap key checked like the other URLs,
    sets the root of the API's links, path kept. Unset, links use the
    request's host, over `http`. `Options.link` builds them; ITEM-0068's
    pages use it.
  - **Tests**, table-driven, with `-race`:
    - the flow ID kept, made, or replaced when it's too long or has
      characters outside the pattern, and its request ID in the JSON log
      line;
    - an incoming `traceparent`'s trace and parent, with the span's name
      and attributes;
    - 404 problems for three unknown paths, 405 problems with `Allow` for
      three methods, and the redirect passing through;
    - the counter's series, checked exactly, and one duration series for
      each operation;
    - the route-to-operation map, links with and without
      `server.public_url`, and the 400 and 500 problem writers.
  - **Generated references:** the configuration and command-line
    references gain `server.public_url`, and the metrics reference the two
    API metrics.
  - **Docs:** "Service endpoints" describes the problems, flow IDs and
    `traceparent` under `/api`. The CHANGELOG's API line covers them.
