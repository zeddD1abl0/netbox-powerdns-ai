---
id: ITEM-0031
title: Share one HTTP client between the NetBox and PowerDNS clients
type: task # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M02
requirements: [REQ-028, REQ-040, REQ-041]
depends_on: []
created: 2026-10-06
closed:
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

- [ ] `internal/httpclient` builds the HTTP client (TLS 1.2 or later, system roots plus a CA file, an optional client certificate and key, no redirects, `tracing.Transport`) and does GETs with the retry policy, the body cap and the `http://` warning.
- [ ] `internal/netbox` uses it, keeping its own headers, error mapping and endpoints. Its tests pass unchanged, apart from transport tests that move with the code.
- [ ] A client certificate is tested against a local TLS server that requires one, and a missing or mismatched certificate and key fail clearly.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Created from M02's approved design.
