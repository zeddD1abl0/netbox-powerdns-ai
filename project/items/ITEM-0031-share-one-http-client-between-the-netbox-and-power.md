---
id: ITEM-0031
title: Share one HTTP client between the NetBox and PowerDNS clients
type: task # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M02
requirements: [REQ-028, REQ-040, REQ-041]
depends_on: []
created: 2026-10-06
closed: 2026-10-06
---

# ITEM-0031: Share one HTTP client between the NetBox and PowerDNS clients

## Goal

The PowerDNS client needs everything the NetBox client's transport already
does: TLS with a CA file, no redirects, `traceparent`, GETs retried with
backoff and `Retry-After`, a body cap, and the `http://` warning. Move that
out of `internal/netbox` into `internal/httpclient`, shared by both, with no
change in the NetBox client's behavior, and add the optional client
certificate (mTLS) that ADR-0024's reference setup can require.

## Acceptance criteria

- [x] `internal/httpclient` builds the HTTP client (TLS 1.2 or later, system roots plus a CA file, an optional client certificate and key, no redirects, `tracing.Transport`) and does GETs with the retry policy, the body cap and the `http://` warning.
- [x] `internal/netbox` uses it, keeping its own headers, error mapping and endpoints. Its tests pass unchanged, apart from transport tests that move with the code.
- [x] A client certificate is tested against a local TLS server that requires one, and a missing or mismatched certificate and key fail clearly.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Created from M02's approved design.
- 2026-10-06: Done.
  - `internal/httpclient` holds what `internal/netbox/client.go` did for
    transport: TLS 1.2 or later with a CA file, now an optional client
    certificate and key, no redirects, `tracing.Transport`, GETs retried
    with the same policy, the 64 MiB body cap, and reading the body before
    decoding. `Get` retries; `GetOnce`, which the NetBox client's
    permission probe uses, doesn't. A failure is an `*UnreachableError`
    (which names the service), a `*StatusError` (status, path, headers and
    the start of the body), or a decoding error. Errors about the TLS files
    name their config keys under `Options.Keys`, such as
    `netbox.ca_file`.
  - `internal/netbox` keeps its headers, endpoints, warnings and error
    mapping (`check` turns a `StatusError` into an `APIError`, a redirect's
    detail, or `denied`). `netbox.UnreachableError` is an alias of the
    shared type, so callers are unchanged.
  - **Log messages changed.** sloglint requires constant messages, so the
    shared client logs `http request`, `http request failed` and
    `retrying an http request`, with a `service` attribute, instead of
    "request to NetBox" and the like. The integration test that counts the
    records pages matches the new message; nothing in the docs quoted the
    old ones.
  - **Tests:** the NetBox client's tests are unchanged except that
    `testClient` sets an `httpclient.Retry`, and `TestBackoff` and
    `TestRetryAfterHeader` moved with the functions they test. The new
    package's tests cover retries, backoff, `Retry-After`, a body that
    stalls, timeouts, cancellation, headers and `traceparent`, no
    credentials in the logs, TLS (an untrusted certificate, a CA file, TLS
    1.1, https to plain HTTP), a client certificate against a server that
    requires one (presented, and missing), and bad TLS files. `make check`
    and `make test-integration` pass.
