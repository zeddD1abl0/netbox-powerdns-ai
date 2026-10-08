---
id: ITEM-0068
title: Service status and server groups in the API
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M06
requirements: [REQ-046]
depends_on: [ITEM-0067]
created: 2026-10-08
closed: 2026-10-08
---

# ITEM-0068: Service status and server groups in the API

## Goal

`/api/status`, `/api/server-groups` and `/api/server-groups/{group}`,
read from a snapshot of the service's state, with the cursor pagination
every list uses (ADR-0033).

## Acceptance criteria

- [x] `Service.Snapshot` copies the state under its lock, and the API reads it through an interface.
- [x] `/api/status` and both group resources answer as the spec says, validated in table-driven tests.
- [x] Lists page with `limit` and `cursor`, with `self` and `next`, and refuse a bad limit or cursor with a 400.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M06's approved design.
- 2026-10-08: `/api/status` was built in ITEM-0066, as the pipeline's first operation. This item keeps the server groups and the pagination.
- 2026-10-08: Done.
  - **The spec** gains `listServerGroups` and `getServerGroup`; the
    `Limit` (1 to 1000, default 100), `Cursor` and `Group` parameters; the
    400 and 404 responses; and `ServerGroup`, `ZoneCounts`, `DriftPolicy`
    and `ServerGroupPage`, Zalando's page object with `self`, `next` and
    `items`.
    - A group's counts and problem and warning counts are null before its
      first successful read, and `error` is null when there's none.
    - URL examples in flow sequences are quoted: unquoted, a `?` or `&`
      breaks oapi-codegen's YAML parser, though vacuum reads them.
  - **The service:** `service.Primary` became `service.Group`, with the
    group's views and default drift policy. `Service.Groups()` returns
    each group's configuration, its info (the code it shares with
    `/status`, now in one function), and its last successful report.
    - The report is returned without a deep copy: a kept report is never
      changed, only replaced by the next refresh's, which the type's
      comment states.
    - A new test shows the configuration and nothing else before any
      refresh, and a failed group keeping its report.
  - **Pagination (`page.go`):**
    - a cursor is base64url of `{"a": last key, "f": filter}`;
    - `newPage` checks the limit, and refuses a cursor that doesn't
      decode, has no key, or was made with another filter;
    - `take` cuts the page;
    - `links` builds the absolute `self`, the request's own URL, and
      `next`, with the cursor and the limit.

    Groups page in the configuration's order, by the name the cursor
    names. A name that's no longer configured is a 400.
  - **A strict middleware**, `withRequest`, puts the request in the
    handlers' context, for links and problems.
  - **A middleware bug, found here:** the handler ran the handler that
    `mux.Handler` returned, which skips the mux's own `ServeHTTP`, the one
    that sets path values, so `{group}` was empty. Matched routes now go
    through the mux.
  - **Tests**, table-driven, with every response checked against the spec:
    - the list, including nulls for a group never read;
    - pages followed by their `next` links at limits 1, 2, 3 and 1000, and
      an empty list;
    - seven bad requests: limits 0, 1001 and `many`, an undecodable
      cursor, an empty one, a vanished group, and another filter;
    - one group, and a 404 for an unknown one;
    - links under `server.public_url`.

    `TestServeWithoutNetBox` sees both groups `unknown`. `TestServe`, on
    the lab, sees lab-a `ok` with its drift counts, `down` failed with
    null counts, and `GET /api/server-groups/down`.
  - **Docs:** "Service endpoints" lists the API as one `/api/…` row, and
    the CHANGELOG's API line covers the groups and the paging.
