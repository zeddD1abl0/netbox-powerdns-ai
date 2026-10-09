---
id: ITEM-0079
title: The lab's NetBox worker and the webhook tests
type: task # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M07
requirements: [REQ-036, REQ-048]
depends_on: [ITEM-0077]
created: 2026-10-08
closed: 2026-10-08
---

# ITEM-0079: The lab's NetBox worker and the webhook tests

## Goal

The lab's optional NetBox worker, a compose profile, `webhooks`; the
integration test that replays signed NetBox 4.7 payloads against `serve`
on the lab in CI; and `make test-webhooks`, a local-only end-to-end test
with real deliveries.

## Acceptance criteria

- [x] `make lab-up LAB_PROFILES=webhooks` adds NetBox's worker, and CI never starts it.
- [x] The replayed integration test sees a record change's zone refreshed through the API within seconds, a view event make a full refresh, and a wrong signature change nothing.
- [x] `make test-webhooks` passes locally, with NetBox's own deliveries.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from M07's approved design.
- 2026-10-08: The compose profile `webhooks` adds `netbox-worker-47`:
  the NetBox image, with the plugin installed as NetBox's is, running
  `manage.py rqworker high default low` once NetBox is healthy, with
  `host.docker.internal:host-gateway`. `LAB_PROFILES` adds profiles to
  every lab command, and `lab-down` takes every profile. The worker used
  282 MiB.
- 2026-10-08: `TestServeWebhooks` (tag `integration`, so CI runs it) runs
  `serve` on a drift fixture with a secret and an hour's interval, changes
  the in-sync zone's www A in NetBox, and posts that change's event, built
  from the captured `record-updated.json`, signed. The zone drifts with one
  change, the group counts two drifted zones, the status and logs carry
  the request ID, and only one full refresh ran. A forged event is a 401
  and refreshes nothing, and a view's event makes a full refresh. 7.9 s
  alone; `make test-integration` took 43 s for internal/cli.
- 2026-10-08: `make test-webhooks` (tags `integration,webhooks`, not in
  `make ci`) starts the lab with the profile, has serve listen on every
  interface, creates a webhook and an event rule in the lab's NetBox, and
  changes a record: NetBox's worker sent `send_webhook` jobs, and the zone
  drifted, in 4.4 s. vet and golangci-lint check the `webhooks` tag too.
  `lab.AdminDo` and `lab.AdminCreate` are exported for these tests.
