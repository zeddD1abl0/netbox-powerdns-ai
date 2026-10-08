---
id: ITEM-0073
title: Fix the M06 code review findings
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M06
requirements: [REQ-046, REQ-047]
depends_on: [ITEM-0072]
created: 2026-10-08
closed: 2026-10-08
---

# ITEM-0073: Fix the M06 code review findings

## Goal

`/code-review high` on `origin/main...m06-rest-api`, at `f4871f1`, found ten
things. This item fixes nine, and records why the tenth stays as designed.

## Acceptance criteria

- [x] An empty NetBox read replaces the last records; a failed read of zones that aren't compared fails nothing else.
- [x] Records come from the view the drift report compares.
- [x] HEAD requests get their route and operation; links keep the path's escapes.
- [x] Zone pages map only their zones; one group is looked up alone; the cursor and zone-name code is shared.
- [x] The vendored license is pinned to a commit; gzip and ETags are negotiated as RFC 9110 says.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Done. The findings, and what changed:
  1. **An empty NetBox read never replaced the last records:** collecting
     an empty sort gives `nil`, which the service read as "not read".
     With `ReadNetBox`, `Report.NetBox` is now never nil, so an empty read
     empties the records. Tests in `drift` and `service` show it.
  2. **HEAD requests lost their route:** a GET pattern answers HEAD, and
     trimming `HEAD ` off `GET /api/…` left it whole, so `/api/docs`
     counted as `openapi`. The route is now the pattern after its method.
     `TestHead` checks the span's name, `http.route`, and both operations'
     counts.
  3. **Links lost a zone name's escapes:** they were built from the
     decoded path, so RFC 2317's `0/25.2.0.192.in-addr.arpa` paged to a
     404. They now use the escaped path. The contract checker also splits
     paths as the mux does, on the escaped path. `TestEscapedZoneLinks`
     follows such a zone's changes page by page.
  4. **Records could come from another view than the report's:** the
     records kept only active zones, so a zone inactive in the view the
     report compares, and active in another, took the other's records.
     `Run` now keeps inactive zones too, bare, and `NetBoxView.In` takes
     the first view by name that has the zone, as `Compare` does, and
     finds nothing if it's inactive there. Tests in `service` and `api`
     cover it.
  5. **A failed read of a zone that isn't compared failed the refresh:**
     `Run` now reads those zones in a call of their own, after the
     comparison's, and reports its failure as `Report.NetBoxErr`. The
     service logs it and keeps NetBox's last records; the comparison
     stands. A test makes one such zone fail.
  6. **Zone lists mapped every zone before paging, counting each zone's
     records by building them all, and every lookup of one group built
     every group:**
     - lists now keep each zone's report, filter, page, and map only the
       page;
     - counts don't allocate;
     - the views are sorted once per request;
     - `Service.Group` builds one group, and the API looks groups up with
       it. Its test checks it against `Groups()`.
  7. **Duplicated code:** `zoneName`, `startAfter`, a generic "start after
     the cursor's item", and `compareRRset` replace the copies in the
     zones, changes and records handlers.
  8. **The license came from Scalar's moving `main`:** the repository tags
     no package's releases, so `make vendor-scalar` now fetches it from a
     pinned commit, `854b0f44…`, checked by the same SHA-256. Re-running
     it changed no file.
  9. **Links use the request's Host when `server.public_url` isn't set:**
     no change. It's ADR-0033's design, documented in the configuration
     reference. Every API response is `Cache-Control: no-store`, so a
     forged Host misleads only the request that sent it. Behind a proxy,
     `server.public_url` is the setting to use.
  10. **gzip and ETags:**
      - `Accept-Encoding` is read with its q-values, so `gzip;q=0` gets
        the plain bundle;
      - `If-None-Match` matches lists, weak tags, and `*`;
      - table tests cover both.

  `/security-review` found nothing at its bar. Its one note below it, DNS
  rebinding against the unauthenticated listener, is ITEM-0074, for M10.
