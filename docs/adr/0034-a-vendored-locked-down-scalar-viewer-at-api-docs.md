---
title: "0034: A vendored, locked-down Scalar viewer at /api/docs"
status: accepted
date: 2026-10-08
decision-makers: [jordan]
requirements: [REQ-018, REQ-046]
questions: []
supersedes:
---

# 0034: A vendored, locked-down Scalar viewer at /api/docs

## Context and problem statement

ADR-0012 says that the binary serves the API's reference with a vendored
viewer. The project vendors UI assets, and uses no CDN (ADR-0022). The
viewer must render OpenAPI 3.1, run in the running service, and send
nothing to any other host: the spec names the API's resources, and the
service runs on trusted networks.

## Decision drivers

- Vendored and embedded, with no CDN (ADR-0022).
- No request from the page to any other host, whatever the viewer's
  defaults.
- OpenAPI 3.1.
- A licence of MIT, BSD, Apache-2.0 or MPL-2.0.
- Useful to people now. AI clients and a later MCP server read the spec,
  served at `/api/openapi.yaml`, not the viewer.

## Considered options

1. Scalar's API reference.
2. Swagger UI.
3. Redoc.
4. No viewer: only the spec, and the reference on the docs site.

## Decision outcome

The user chose Scalar on 2026-10-08, locked down, after comparing it with
Swagger UI.

**The asset:**
- Scalar's API reference (MIT), the standalone bundle from
  `@scalar/api-reference`.
- It's vendored into `internal/api/docs/`, with its licence, and the
  binary embeds it.
- `make vendor-scalar` fetches a pinned version, and checks it by SHA-256.

**Locked down:**
- A same-origin init script sets `withDefaultFonts: false`, which
  otherwise loads fonts from Scalar's CDN. It turns Agent Scalar off,
  which otherwise uploads the spec to Scalar's hosted service, and points
  at `/api/openapi.yaml`.
- The page and its assets are served with a strict Content-Security-Policy:
  - `default-src 'none'`;
  - `script-src`, `style-src`, `font-src`, `img-src` and `connect-src`
    limited to `'self'`, with `'unsafe-inline'` for styles only if Scalar
    needs it;
  - `frame-ancestors 'none'`.

  A test checks the header. The browser refuses any request to another
  host, even from a default that a later Scalar release adds.

### Consequences

- Good: a modern reference, with multi-language code samples and a
  request client, served by the binary itself.
- Good: nothing leaves the page, which the browser enforces through the
  CSP, not only Scalar's settings.
- Bad: a bundle of about 3 MB in the binary and the repository. Scalar
  releases often, so the vendored copy is updated deliberately, through
  `make vendor-scalar`, not on every release.
- Bad: with Scalar's fonts off, the page uses the browser's own.

### Confirmation

- A test checks the CSP header on `/api/docs` and its assets, and that
  the embedded page references only same-origin URLs.
- M06's manual verification loads the page in a headless browser.

## Pros and cons of the options

### Swagger UI

- Good: mature and familiar, slower to change, about 1.5 MB, with "Try it
  out" and "Authorize".
- Bad: plainer navigation, curl-only code samples. By default it sends
  the spec's URL to swagger.io's online validator.

### Redoc

- Good: the smallest, about 1 MB, and read-only.
- Bad: no request client.

### No viewer

- Good: nothing to vendor or update.
- Bad: no browsable reference in the running service.

## More information

- ADR-0012 (the API standard), ADR-0022 (the toolchain), ADR-0033 (the
  API).
- Recorded in M06's design, 2026-10-08. Implemented by ITEM-0071.
