---
id: ITEM-0079
title: The lab's NetBox worker and the webhook tests
type: task # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M07
requirements: [REQ-036, REQ-048]
depends_on: [ITEM-0077]
created: 2026-10-08
closed:
---

# ITEM-0079: The lab's NetBox worker and the webhook tests

## Goal

The lab's optional NetBox worker, a compose profile, `webhooks`; the
integration test that replays signed NetBox 4.7 payloads against `serve`
on the lab in CI; and `make test-webhooks`, a local-only end-to-end test
with real deliveries.

## Acceptance criteria

- [ ] `make lab-up LAB_PROFILES=webhooks` adds NetBox's worker, and CI never starts it.
- [ ] The replayed integration test sees a record change's zone refreshed through the API within seconds, a view event make a full refresh, and a wrong signature change nothing.
- [ ] `make test-webhooks` passes locally, with NetBox's own deliveries.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M07's approved design.
