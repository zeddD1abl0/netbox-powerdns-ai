---
id: ITEM-0053
title: nbpdns serve
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M04
requirements: [REQ-003, REQ-014, REQ-044]
depends_on: [ITEM-0051, ITEM-0052]
created: 2026-10-07
closed: 2026-10-07
---

# ITEM-0053: nbpdns serve

## Goal

`nbpdns serve`: refresh the drift report on a schedule, keep each group's
last-known state, answer `/livez` and `/readyz`, expose `/metrics` with
the drift metrics, and shut down cleanly (ADR-0029).

## Acceptance criteria

- [x] `server.listen`, `drift.interval` (at least 10s) and `drift.timeout` load and validate.
- [x] Refreshes start on schedule and never overlap, each one a trace with its own request ID; a failed group or NetBox keeps its last-known counts, with `up` at 0.
- [x] `/readyz` and `/livez` follow ADR-0029, and SIGTERM stops the service cleanly, exiting 0.
- [x] Unit tests and integration tests against the lab cover the schedule, the state, the endpoints and the drift metrics.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-07: Created from M04's approved design.
- 2026-10-07: Done.
  - **Keys:** `server.listen` (`:8080`, a TCP address), `drift.interval`
    (`5m`, at least `10s`, through a new `durationKeyAtLeast`) and
    `drift.timeout` (`10m`).
  - **`internal/service`:** `Run` refreshes at once, then every interval
    from start to start; an overrun delays the next refresh, with a
    warning, so refreshes never overlap. Each refresh is a new root span,
    `drift refresh`, with its own request ID, though `serve`'s command span
    is the context's, and is bounded by `drift.timeout`; one cut short by
    shutdown isn't recorded. `record` sets the outcome (failed if NetBox
    couldn't be read, incomplete if a group couldn't), the refresh metrics,
    and each group's last-known state. A failed group keeps its counts and
    drifted zones with `up` at 0; a failed NetBox keeps every group's, and
    their `up`, since that refresh doesn't try the primaries
    (`nbpdns_server_group_up`'s help now says so). A drifted zone's series
    is removed only when the zone stops drifting, never cleared and set
    again, so no scrape misses one that stays. It logs a line per group
    per refresh, warnings for failures and the report's warnings, and each
    drifted zone at debug. `/livez` and `/readyz` follow ADR-0029; the mux
    gives 405 and 404.
  - **`nbpdns serve`** builds the NetBox client and each group's client
    once, with request observers, and logs a group without one, which every
    refresh then reports failed. Each refresh runs `compare`, the code
    `nbpdns drift` now shares: NetBox's `Connect`, then `drift.Run`, with
    each primary connecting in its own first listing, so groups connect
    concurrently too. It listens, logs `serving` with the address, runs
    the service until the context is canceled (SIGINT or SIGTERM) or the
    listener fails, then drains the HTTP server for up to 10 seconds, and
    exits 0. The HTTP server has read-header, read, write and idle
    timeouts.
  - **Tests:** the schedule and an overrun, shutdown during a refresh, a
    refresh past its timeout, readiness and liveness with a fake clock,
    HEAD, 405 and 404, last-known state through four refreshes, and one
    trace and request ID per refresh, in `internal/service`; in
    `internal/cli`, serve without groups, on an address in use, and
    against an unreachable NetBox (ready after its failed first refresh,
    `nbpdns_netbox_up` 0, exit 0 on cancel). `TestServe`, against the lab
    with the drift fixture and a group that refuses connections, checks
    every fixture case's counts and drifted-zone series, the change kinds,
    each group's `up`, the request counts, and a clean stop.
  - **Binary size:** 28,215,222 bytes, from 26,338,560 after ITEM-0052:
    `client_golang` and the service add about 1.9 MB.
