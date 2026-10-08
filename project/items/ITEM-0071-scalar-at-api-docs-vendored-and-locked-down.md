---
id: ITEM-0071
title: Scalar at /api/docs, vendored and locked down
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M06
requirements: [REQ-018, REQ-046]
depends_on: [ITEM-0066]
created: 2026-10-08
closed: 2026-10-08
---

# ITEM-0071: Scalar at /api/docs, vendored and locked down

## Goal

Scalar's API reference at `/api/docs`, vendored and embedded, with
its fonts and Agent Scalar off, and a strict same-origin
Content-Security-Policy, so the page reaches no other host (ADR-0034).

## Acceptance criteria

- [x] `make vendor-scalar` fetches a pinned version, checks its SHA-256, and writes it and its licence to `internal/api/docs/`.
- [x] `/api/docs` and its assets are served with the CSP, and the embedded page references only same-origin URLs, as tests show.
- [x] The page renders the reference in headless Firefox.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M06's approved design.
- 2026-10-08: Done.
  - **Scalar 1.73.1**, `@scalar/api-reference`, MIT, released
    2026-10-07. `make vendor-scalar` fetches its npm tarball, checks it by
    SHA-256 (`424d2f1e…`; npm's sha512 integrity matched too), and takes
    `dist/browser/standalone.js`. That file is a self-contained 4.4 MB
    script; the `chunks/` beside it belong to the ESM build. It's gzipped
    with `-9 -n`, so the same input gives the same 1.28 MB file, as
    `internal/api/docs/scalar.js.gz`. The npm package has no licence file,
    so `LICENSE.scalar` comes from Scalar's repository, checked by SHA-256
    too.
  - **The page:** `/api/docs`, with relative URLs only, `docs/scalar.js`,
    `docs/init.js` and `openapi.yaml`, so it works under a proxy's path
    too. `init.js` sets:
    - `url: "openapi.yaml"`;
    - `withDefaultFonts: false`;
    - `agent: {disabled: true}`;
    - `mcp: {disabled: true}`;
    - `telemetry: false`;
    - `showDeveloperTools: "never"`.

    The option names come from the bundle's own schema.
  - **The policy:** `default-src 'none'`; `script-src`, `connect-src`,
    `font-src` and `img-src` limited to `'self'`, with `data:` for fonts
    and images; `style-src 'self' 'nonce-…'`; `base-uri` and
    `form-action 'none'`; `frame-ancestors 'none'`. It comes with
    `nosniff`, `no-referrer` and `X-Frame-Options: DENY`.
    - Scalar reads a `<meta property="csp-nonce">`, and puts it on the
      style it injects, so the page gets a new nonce in each response, and
      no `'unsafe-inline'` was needed.
    - The bundle has no `eval`, no `new Function` and no workers.
  - **Checked in headless Firefox** (an isolated profile), on `serve`
    against the lab: the reference rendered styled, with the spec's
    operations, models and server URL, and the browser's own fonts.
  - **Assets:**
    - `scalar.js` is served gzipped to clients that accept it, and
      decompressed for others, with `Vary: Accept-Encoding`;
    - each asset has an ETag per encoding, and `Cache-Control: no-cache`,
      so a browser revalidates it, and gets a 304, rather than downloading
      1.28 MB again;
    - the page itself stays `no-store`;
    - an unknown asset is a 404 problem.
  - **Metrics:** requests for the reference count as operation `docs`.
  - **Tests:**
    - the page's nonce matches the policy's and changes per response, the
      policy has no `unsafe`, and the page names no other host;
    - `scalar.js` is the same gzipped or not, with distinct ETags, and
      revalidates to a 304;
    - `init.js` has each setting, and the licence is served;
    - 404 problems for an unknown file and for `/api/docs/`;
    - the metrics count `docs`.
  - **Size:** the stripped binary goes from 19.8 MB to 21.1 MB, +1.31 MB
    for the gzipped bundle.
  - **Docs:** "Service endpoints" lists `/api/docs`, and the CHANGELOG has
    it.
