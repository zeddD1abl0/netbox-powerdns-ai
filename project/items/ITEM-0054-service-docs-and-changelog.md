---
id: ITEM-0054
title: Service docs and CHANGELOG
type: task # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M04
requirements: [REQ-014]
depends_on: [ITEM-0053, ITEM-0055]
created: 2026-10-07
closed: 2026-10-07
---

# ITEM-0054: Service docs and CHANGELOG

## Goal

The M04 docs: the tutorial "Run nbpdns as a service", the how-tos
"Monitor drift with Prometheus" and "Export traces to an OpenTelemetry
collector", the explanation "How nbpdns runs as a service", the reference
"Service endpoints", the generated references, and the CHANGELOG.

## Acceptance criteria

- [x] The tutorial, how-tos, explanation and reference exist, each checked against the lab.
- [x] The generated references are current, and `CHANGELOG.md` has lines under Unreleased.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-07: Created from M04's approved design.
- 2026-10-07: Done. New pages: the tutorial "Run nbpdns as a service"
  (a zone on both sides, `serve` at a 10-second interval, `/readyz`,
  `/livez`, `/status` in both forms, the metrics, a change on PowerDNS seen
  by the next refresh, `nbpdns drift` for the change, and Ctrl+C), the
  how-tos "Monitor drift with Prometheus" (a scrape job and five alert
  rules: a drifted zone, a group down, NetBox down, no complete refresh
  for three intervals, and nbpdns down) and "Export traces to an
  OpenTelemetry collector" (both protocols, a CA file, headers from a
  file), and the explanation "How nbpdns runs as a service" (the
  schedule, last-known state and why, readiness and liveness and why,
  the metrics' shape and cardinality, the status page, the listener's
  exposure, and what isn't done yet). The reference "Service endpoints"
  came with ITEM-0055, under that name rather than "HTTP endpoints"; the
  metrics reference with ITEM-0051. The indexes list them, and the
  CHANGELOG has a documentation line. Checked against the lab: the
  tutorial's steps ran on a fresh lab (`make lab-down`, then `make
  lab-up`), and its outputs are theirs, with IDs shortened; the how-to's
  rules pass `promtool check rules` from `prom/prometheus:v3.7.2`, with the
  output the page shows. Prometheus, Grafana and Alertmanager are in the
  Vale vocabulary, which also exempts them from the heading rule.
