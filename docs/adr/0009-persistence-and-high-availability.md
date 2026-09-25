---
title: "0009: Persistence and high availability — PostgreSQL and SQLite"
status: accepted
date: 2026-09-25
decision-makers: [jordan]
requirements: [REQ-002, REQ-003, REQ-032, REQ-033]
questions: [Q-019, Q-020]
---

# 0009: Persistence and high availability — PostgreSQL and SQLite

## Context and problem statement

The app stores its own state:
- configuration and runtime settings;
- users, roles and tokens;
- audit events and the delivery outbox;
- sync plans and drift state.

It must run as a single binary (REQ-002) and as a container or pod (REQ-003). The
second form usually implies more than one replica.

## Decision drivers

- A single binary should work without any external service.
- Container deployments should be able to run several replicas without two of
  them writing to the same DNS server at once.
- One schema and one set of migrations to maintain.

## Considered options

1. PostgreSQL and SQLite.
2. PostgreSQL only.
3. SQLite only, single instance.

## Decision outcome

Chosen option: **PostgreSQL and SQLite** (Q-019, Q-020).

| Mode | Database | Instances |
|---|---|---|
| Single binary, small or lab installs | Embedded SQLite (a pure-Go driver, so no CGO) | Exactly one |
| Container, pod or production | PostgreSQL | One or more replicas |

- **One schema, one migration set** works on both. SQL that differs between the
  two is kept to a documented minimum, and CI runs the full test suite against
  both.
- **HA on PostgreSQL:**
  - every replica serves the UI and API (active-active);
  - exactly one replica runs the sync worker at a time, chosen by leader
    election;
  - the intended mechanism is a PostgreSQL advisory lock, with no extra
    infrastructure. It's confirmed in M1 design.
- **SQLite mode refuses to start** if it detects another instance using the
  same database, so it can't split-brain by accident.

### Consequences

- Good: the single binary needs nothing else. Production can scale the API and
  fail over the sync worker.
- Bad: two SQL dialects. Every data-access change is tested against both, and
  the query layer must support both. The library choice is an M1 decision.
- Bad: the SQLite mode has no HA. If the instance restarts, the control plane
  is down briefly. DNS keeps serving, because the app is never in the DNS data
  path (Q-027).

### Confirmation

- CI runs the test suite against both databases.
- An integration test starts two replicas on PostgreSQL and checks that only
  one of them runs sync. A second test starts two SQLite instances on the same
  file and checks the second refuses to start.
