---
id: ITEM-0057
title: Harden OTLP export and URL errors after the M04 security review
type: task # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M04
requirements: [REQ-012]
depends_on: [ITEM-0056]
created: 2026-10-07
closed: 2026-10-07
---

# ITEM-0057: Harden OTLP export and URL errors after the M04 security review

## Goal

`/security-review` of M04 found nothing at its bar, but two cheap
defense-in-depth items: the OTLP/HTTP exporter follows redirects, which
nbpdns's own clients never do, so a redirect would carry `otlp.headers` to
wherever it points; and `checkURL` quotes the whole URL in its scheme error,
credentials and all.

## Acceptance criteria

- [x] The OTLP/HTTP exporter follows no redirect, with a test of a collector that redirects.
- [x] No URL check quotes a value that holds credentials.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-07: Done.
  - **Redirects:** the OTLP/HTTP exporter now gets nbpdns's own
    `http.Client`, whose `CheckRedirect` answers `http.ErrUseLastResponse`,
    as nbpdns's other clients do, so a 3xx is a failed export rather than
    the headers sent on. `WithHTTPClient` overrides the exporter's TLS and
    timeout options, so the client carries the transport's TLS config and
    `otlp.timeout`. A test exports to a collector that answers 307: the
    export fails, and the target gets nothing. The gRPC exporter doesn't
    follow redirects.
  - **`checkURL`** checks for credentials before the scheme, and never
    quotes a value with an `@` in it. `TestLoadErrors` now fails any error
    that shows `s3cret`, and has a URL with credentials and a `grpc://`
    scheme; with the old check, that case fails.
  - **Left as they are, LOW and by design:** the unauthenticated listener
    on all interfaces, which a DNS-rebinding page on the trusted network
    could read (documented, until M10; nothing on it changes state or holds
    a secret), and upstream error text with control characters on the text
    status page, which needs control of NetBox's or a primary's answers.
