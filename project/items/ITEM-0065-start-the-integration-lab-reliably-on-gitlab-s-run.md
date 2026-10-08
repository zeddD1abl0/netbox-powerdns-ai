---
id: ITEM-0065
title: Start the integration lab reliably on GitLab's runner
type: bug # feature | bug | debt | task
status: wontfix # open | in-progress | blocked | done | wontfix
milestone: M06
requirements: [REQ-036]
depends_on: []
created: 2026-10-08
closed: 2026-10-08
---

# ITEM-0065: Start the integration lab reliably on GitLab's runner

## Goal

On GitLab's runner, the lab's NetBox sometimes isn't healthy in time, so
`make lab-up` fails, and `integration-test` with it, before any test runs.
A retry has always passed so far. This item finds why NetBox's first start
is so slow there, and makes `lab-up` reliable, so that pipelines don't
need retrying.

## Acceptance criteria

- [ ] The cause of the slow NetBox start on GitLab's runner is known and recorded.
- [ ] `integration-test` passes first time in a run of GitLab pipelines, and `lab-up`'s time is recorded.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-08: Created from the v0.1.0 tag pipeline, 780, read with the
  read-only project token.
  - **Job 4229** started NetBox at 23:59:56Z. `docker compose up --wait`
    gave up at 00:19:56Z, after its 1200-second `--wait-timeout`, with
    "timeout waiting for dependencies". The retry, 4236, passed: NetBox
    was healthy 10m28s after starting.
  - **Job 4191**, in pipeline 776 on `m05-packaging`, failed the same way.
    NetBox was marked unhealthy at 16m17s, just past its healthcheck's
    900-second `start_period` and three 30-second retries. Its retry,
    4197, passed.
  - Of the last four pipelines, 776, 777, 779 and 780, two failed
    `integration-test` on the first try. On GitHub, the whole job takes
    about seven minutes.
  - **Not the cause:** `main`'s pipeline, 779, wasn't running its lab at
    the same time; it finished half an hour before 4229 started.
  - **Leads:**
    - the NetBox image's first start, migrations and the DNS plugin's
      install, on the runner's CPU and memory;
    - `unit-test` with `-race`, which runs alongside it in the test stage;
    - the Kubernetes node it lands on.

    Raising `start_period` and `--wait-timeout` would hide the cause, not
    fix it.
- 2026-10-08: Won't fix, on the user's word in M06's design:
  > Unfortunately the issue is to do with the hardware restrictions on the
  > runner. This is a known issue that if the node is busy with other
  > actions, the memory paging kicks in, slowing down the NetBox run-up.
  > Avoid trying to fix this, as retrying the pipeline continues to
  > succeed, and I do not have the resources to expand the capacity of the
  > GitLab Runner currently.

  So the cause is memory paging on a busy node. A failed `integration-test`
  whose log ends in `lab-up`'s timeout, or NetBox unhealthy, is retried,
  not investigated.
