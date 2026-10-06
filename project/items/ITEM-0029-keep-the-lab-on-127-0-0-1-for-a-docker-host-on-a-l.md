---
id: ITEM-0029
title: Keep the lab on 127.0.0.1 for a Docker host on a loopback address
type: bug # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M01
requirements: [REQ-036]
depends_on: []
created: 2026-10-06
closed: 2026-10-06
---

# ITEM-0029: Keep the lab on 127.0.0.1 for a Docker host on a loopback address

## Goal

The lab's credentials are published, so its ports listen on `127.0.0.1` on a
local Docker host, and on every interface only for a remote one that the
tests reach by name, as in CI. The Makefile treated every `tcp://`
`DOCKER_HOST` as remote, so a local daemon reached over TCP on a loopback
address, such as `tcp://localhost:2375`, published the lab's NetBox, with its
superuser, on every interface of the developer's host. The lab how-to says
that doesn't happen. Found by `/security-review` at M01's close, below its
reporting bar but a real gap between the docs and the Makefile.

## Acceptance criteria

- [x] A `tcp://` `DOCKER_HOST` on `localhost`, `127.x.x.x` or `[::1]` binds the lab to `127.0.0.1`; any other `tcp://` host still binds `0.0.0.0`, and a socket or no `DOCKER_HOST` binds `127.0.0.1`.
- [x] The lab how-to says so.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Done. `LAB_LOOPBACK` lists the loopback forms, and
  `LAB_BIND_ADDRESS` is `0.0.0.0` only for a `tcp://` host outside them.
  Checked with `make --eval` for an unset `DOCKER_HOST`, a Unix socket,
  `tcp://docker:2375`, `tcp://10.1.2.3:2376`, `tcp://localhost:2375`,
  `tcp://localhost`, `tcp://127.0.0.1:2375`, `tcp://[::1]:2375`,
  `tcp://[fd00::1]:2375` and `tcp://localhost.example.com:2375`: only the
  four remote ones give `0.0.0.0`. `lab.Host()` already returns the host
  of a `tcp://` `DOCKER_HOST`, so the tests reach a loopback one as before.
