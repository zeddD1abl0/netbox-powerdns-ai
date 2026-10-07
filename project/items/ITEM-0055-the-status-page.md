---
id: ITEM-0055
title: The status page
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M04
requirements: [REQ-014]
depends_on: [ITEM-0053]
created: 2026-10-07
closed: 2026-10-07
---

# ITEM-0055: The status page

## Goal

`/status` shows the service's state: the schedule, the refreshes, NetBox
and each group, with their drifted zones, and the trace export. It's
human-readable text by default, and JSON with `?json=1` (ADR-0029).

## Acceptance criteria

- [x] `/status` gives the state ADR-0029 and the plan list, as text by default and as JSON with `?json=1`, before and after the first refresh.
- [x] Every JSON field is in the reference page "Service endpoints", which a test checks, and neither form holds a token, key or header.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-07: Created from M04's approved design.
- 2026-10-07: Done.
  - **`service.Status`** is the status document: the build, start time
    and uptime, readiness and liveness, the schedule (interval, timeout,
    the last refresh with its outcome, the last complete one, the next one,
    and the counts by outcome), NetBox (URL, up, error), each group in the
    config file's order (primary URL; `ok`, `failed`, or `unknown` before
    a refresh tried it; last success; error; counts; drifted zones with
    their state and number of changes; problems; warnings), and the trace
    export. Every field is always present, `null` when there's nothing
    yet, so the JSON only gains fields. It never holds a token, key or
    header: `serve` passes the service only URLs, names and the OTLP
    protocol.
  - **`/status`** answers text by default, with `text/tabwriter` tables,
    since the service can't import the CLI's, and JSON, indented, with
    `?json=1`. Before the first refresh, the text says there's no drift
    report until it finishes. The text ends by pointing at `?json=1`, and
    at `nbpdns drift` for each RRset's changes.
  - **The reference page** is `docs/reference/service-endpoints.md`,
    "Service endpoints" rather than the plan's "HTTP endpoints", since
    Google's heading rule rejects a heading that starts with an acronym;
    M04's file records it. It covers all four endpoints, with the status
    JSON's example and a field table. `TestStatusIsDocumented` decodes the
    example strictly into `Status`, and checks that every leaf field is in
    the table and the example, and that every row names a field; removing a
    row makes it fail.
  - **Tests:** both forms before and after a refresh, with a failed group;
    in `internal/cli`, neither form shows the NetBox token or the OTLP
    headers, with both set; and in `TestServe`, against the lab, the
    fixture's drifted zones and the failed group, in both forms.
