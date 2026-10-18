---
id: M09
title: PostgreSQL and HA
status: in-progress # planned | in-progress | done
started: 2026-10-18
closed:
---

# M09: PostgreSQL and HA

A roadmap entry, written ahead on `plan-m08-m12` (ADR-0037), and designed
at its start, on 2026-10-18, on its branch, `m09-postgresql-and-ha`. Its
ADRs, ADR-0046 to ADR-0048, were accepted with the plan.

## Goal

Run several replicas of `nbpdns serve` on PostgreSQL 18, each serving the
same answers from the database. Exactly one of them, the leader, refreshes
and prunes. When it stops, dies or is cut off, another takes over within
about 30 seconds, and the old one stops leading first. SQLite stays as M08
left it: a single instance.

## Scope

The roadmap's, of 2026-10-09. **Approved design** has the detail.

- **PostgreSQL 18**, with the same migrations and queries as SQLite
  (ADR-0009, ADR-0038): sqlc's second, PostgreSQL package, behind the store's
  interface, and the store's tests run against both databases. More releases
  are added as the CI runners have room, as for NetBox and PowerDNS.
- **The leader:** a PostgreSQL session advisory lock, held on a connection
  of its own. Only the leader runs refreshes, the prune and, from M14, the
  writes; a replica that gets the lock takes over. TCP keepalives bound how
  long a dead leader's lock lasts, to about 30 seconds.
- **Every replica serves the API and status from the database**, not from
  its memory, so that each answers the same.
- **Webhooks on any replica:** a replica that isn't the leader queues the
  zones in the database, for the leader to refresh. This lifts M07's "a
  webhook reaches one replica" limit.
- **Settings and groups changed with the CLI** reach every replica, through
  the settings' version, as in M08.
- **`/status` and the metrics** say which replica leads, and since when.
- **The lab:** a database for nbpdns in the lab's existing PostgreSQL 18,
  which NetBox already runs, so CI needs no more memory.

## Non-goals

- **No move from SQLite:** an install that changes to PostgreSQL starts with
  an empty database (Q-061).
- **No HA on SQLite:** it stays a single instance (ADR-0009).
- **No transaction-mode pooler:** the leader's lock, `LISTEN` and the
  session's settings need a session of their own. nbpdns connects directly,
  or through PgBouncer in session mode, and the docs say so.
- **No multi-region, and no read replicas.**
- **No other PostgreSQL release** until the runners have room (REQ-054).
- **No writes to PowerDNS:** M14 brings them, and its writes follow the
  leader rules set here.

## Dependencies

- M08: the store, its migrations and queries, the audit trail, settings and
  groups in the database, and drift history.

## Decided, 2026-10-09

- PostgreSQL 18, more releases as the runners allow (REQ-054, Q-060).
- No move from SQLite to PostgreSQL (Q-061).
- Leader election by a PostgreSQL advisory lock (Q-062), as ADR-0009
  intended.

## Open for its design

- How the replicas learn of a change of leader quickly: `LISTEN` and
  `NOTIFY`, or polling.
- The replica tests: how a leader is killed in CI, and how long a takeover
  may take there.
- `database.url`'s form for PostgreSQL: TLS, client certificates, and the
  password from a `_FILE`.

The user answered these on 2026-10-14, in the brief: support both,
polling by default, with `LISTEN` and `NOTIFY` preferred; the takeover
test is for the design to work out, such as by stopping the leader's
container; and `database.url` in PostgreSQL's standard URI form.

All three were settled in the design, on 2026-10-18: the replicas poll,
with `LISTEN` and `NOTIFY` as an option (ADR-0048); the takeover tests
run the replicas as child processes, and stop, kill or cut off the
leader (ADR-0047); and
`database.url` takes PostgreSQL's standard URI, with libpq's `sslmode`
default, `prefer`, client certificates through its standard parameters,
and the password from `NBPDNS_DATABASE_URL_FILE` or a `passfile`
(ADR-0046).

## Phases

| Phase | Items |
|---|---|
| M9a Store | ITEM-0110 The store on PostgreSQL: pgx, sqlc's PostgreSQL package and tests on both |
| | ITEM-0111 `database.url` for PostgreSQL: TLS, the URL as a secret, and `serve` and the CLI on it |
| M9b Leader | ITEM-0112 Leader election by a PostgreSQL session advisory lock |
| M9c Replicas | ITEM-0113 The replicas' change feed: polling, and `LISTEN` and `NOTIFY` |
| | ITEM-0114 Every replica serves the API and status from the database |
| | ITEM-0115 Webhooks on any replica, through a queue in the database |
| M9d Testing and docs | ITEM-0116 The takeover tests: stopped, killed and cut-off leaders |
| | ITEM-0117 The M09 docs: several replicas on PostgreSQL, and how the leader is chosen |

ITEM-0109 recorded this design. ITEM-0100, from M08's last pipelines,
closes once a merge and its tag both pass without a retry.

## Acceptance criteria

- [x] ADR-0046 to ADR-0048 are accepted, and the brief records the answers of
  2026-10-18.
- [ ] The store runs on PostgreSQL 18 from the same migrations and queries.
  Its tests pass on both databases, and `generate-check` covers both
  packages.
- [ ] `serve` and the CLI take a `postgres://` URL in standard form, with TLS
  and client certificates as libpq has them. The URL can come from
  `NBPDNS_DATABASE_URL_FILE`, or its password from a `passfile`. Its
  password is never logged or shown.
- [ ] With several replicas on one database, exactly one refreshes and
  prunes.
- [ ] When the leader stops or is killed, another replica takes over within
  seconds. When it's cut off, another takes over within about 30 seconds,
  and the old one stops leading first.
- [ ] Every replica serves the same API answers and deployment status from
  the database: within the poll interval, or at once with `database.listen`.
- [ ] A webhook sent to any replica is refreshed by the leader, and survives
  a takeover. Settings and groups changed with the CLI reach every replica.
- [ ] `/status` and the metrics say which replica leads, and since when. A
  takeover is an audit event.
- [ ] SQLite behaves as in M08, a single instance, apart from serving
  committed refreshes only.
- [ ] `make check`, `make test-integration` and `make release-check` pass,
  and the pipelines pass within the runner's memory.
- [ ] ITEM-0100 is closed: M09's merge and tag pipelines pass without a
  retry.
- [ ] The docs exist, the references are regenerated, and the CHANGELOG is
  updated.
- [ ] `/code-review high` and `/security-review` have run. The security
  review covers the database's credentials, TLS and the audit trail.
- [ ] The manual verification is recorded. The user has merged through an MR
  with a merge commit, and tagged `v0.9.0`.

## Verification log

<!-- Append-only. Start each entry with the date: "- YYYY-MM-DD: …" -->

## Approved design

The plan approved on 2026-10-18, copied verbatim. Its headings are demoted two
levels to nest under this section; the text is unchanged. It's a snapshot.
Where it disagrees with an ADR or `CLAUDE.md`, they win.

### M09: PostgreSQL and HA

#### Context

M08 gave `nbpdns serve` a database, embedded SQLite, with one instance on
it. Its SQL was written to run on PostgreSQL too, and `sqlc compile` already
checks that PostgreSQL takes it. M09 adds PostgreSQL 18 and runs several
replicas of `serve` on it, one of them the leader (REQ-033, REQ-054,
ADR-0009). The roadmap, written on 2026-10-09, left three points to this
design. The user answered them on 2026-10-14 and 2026-10-18.

What the code holds today, and what it means for M09:

- **The store** calls sqlc's SQLite package, `internal/store/sqlite`, 50
  queries. Generated for PostgreSQL, the package differs only in its
  placeholders, and in `LIMIT`'s parameters, `int32` instead of `int64`.
- **Writes** rely on SQLite's immediate transactions, which run one at a
  time. The audit trail reads its last event, then appends the next one.
  On PostgreSQL, two writers could do that at once, and one would fail.
- **The API and `/status`** are built from `internal/service`'s memory. Only
  drift history is read from the store. NetBox's zones, which the API's
  records come from, aren't stored at all.
- **Webhooks' zones** wait in memory, in `Service.pending`. A restart loses
  them.
- **Settings and groups** reach a running `serve` through a 5-second poll
  of their versions (`internal/cli/live.go`, `watcher.run`).
- **The lab** runs PostgreSQL 18 for NetBox, as `postgres-47`, with no port
  published.

#### Goal

Run several replicas of `nbpdns serve` on PostgreSQL 18, each serving the
same answers from the database. Exactly one of them, the leader, refreshes
and prunes. When it stops, dies or is cut off, another takes over within
about 30 seconds, and the old one stops leading first. SQLite stays as M08
left it: a single instance.

#### Non-goals

- **No move from SQLite:** an install that changes to PostgreSQL starts with
  an empty database (Q-061).
- **No HA on SQLite:** it stays a single instance (ADR-0009).
- **No transaction-mode pooler:** the leader's lock, `LISTEN` and the
  session's settings need a session of their own. nbpdns connects directly,
  or through PgBouncer in session mode, and the docs say so.
- **No multi-region, and no read replicas.**
- **No other PostgreSQL release** until the runners have room (REQ-054).
- **No writes to PowerDNS:** M14 brings them, and its writes follow the
  leader rules set here.

#### Decisions

Approving this plan accepts these ADRs. M09 starts now, so they aren't
written ahead as proposals (ADR-0037).

##### ADR-0046: the store on PostgreSQL 18, with pgx and a second sqlc package

- **Driver:** jackc/pgx v5 (MIT), through its `database/sql` adapter, so that
  both dialects use `database/sql` and sqlc's `database/sql` code. lib/pq is
  in maintenance mode.
- **Queries:** sqlc generates `internal/store/postgres` from the same SQL.
  `sqlc-postgres.yaml` changes from `compile` to `generate`, and
  `generate-check` diffs both packages. An override maps PostgreSQL's `int4`
  to `int64`, so that the two packages' structs have the same fields. The
  store calls a `querier` interface in the SQLite package's types. An
  adapter converts the PostgreSQL package's types, and the compiler checks
  each conversion.
- **Migrations:** the same goose files, with goose's PostgreSQL dialect and
  its session locker, an advisory lock, so that replicas starting together
  migrate once. Migrations 00001 to 00005 don't change.
- **Writes, one at a time:** on PostgreSQL, each write transaction first
  takes a transaction-scoped advisory lock, as SQLite's immediate
  transactions wait for each other. The audit chain's numbers and hashes,
  the sequences, and M08's rotation and rename keep their guarantees.
- **Order:** the store sorts names in Go, byte by byte, so that lists come in
  the same order on both databases, whatever the database's collation.
  Times are stored in a fixed-width form, so they compare correctly as text
  in any collation.
- **`database.url`:** `postgres://` or `postgresql://`, PostgreSQL's
  standard URI. pgx reads libpq's parameters and defaults: `sslmode`, by
  default `prefer` (the user's choice); `sslrootcert`, `sslcert`, `sslkey`
  and `sslpassword` for TLS and client certificates; `passfile`; and a Unix
  socket through `host=`. It also reads libpq's `PG*` environment variables,
  below the URL's own values.
- **The URL is a secret:** it can come from `NBPDNS_DATABASE_URL_FILE`, and
  nbpdns never logs or shows its password. `/status` shows the URL without
  the password, and whether the connection uses TLS. The docs recommend
  `sslmode=verify-full`.
- **Tests:** the store's tests run on SQLite in `make test`. In `make
  test-integration` they also run on PostgreSQL, each test in a fresh
  database in the lab's PostgreSQL 18, which the test drops afterwards.
- **The lab:** `make lab-up` creates an `nbpdns` role and database in
  `postgres-47` if they're missing, which works on an existing volume too.
  It publishes the database's port on the lab's bind address.

##### ADR-0047: the leader, by a PostgreSQL session advisory lock

- **The lock:** `pg_try_advisory_lock`, on a key derived from the database's
  ID, so that two installs never share it. It's held on a connection of its
  own, which never goes through a transaction-mode pooler.
- **How long a lost leader's lock lasts:** the lock's session sets these
  options. PostgreSQL then ends a dead leader's session, and with it the
  lock, within about 25 seconds, and a hung leader's within 30.
  - `tcp_keepalives_idle=10`, `tcp_keepalives_interval=5` and
    `tcp_keepalives_count=3`, for a dead host;
  - `tcp_user_timeout=25s`, for data that the leader never acknowledges;
  - `idle_session_timeout=30s`, for a hung process, whose kernel still
    answers the keepalives.
- **Heartbeat and step-down:** every 5 seconds, the leader updates its row
  on the lock's connection, with a 5-second deadline. A leader that misses
  one steps down: it cancels its refresh, stops the prune, closes the
  connection, and becomes a follower. It steps down within 10 seconds, so
  before PostgreSQL can free the lock, and two leaders never overlap.
- **A fence on writes:** leader-only writes check, in their transaction, that
  the lock's session, by its backend PID in `pg_locks`, still holds the
  lock. A deposed leader that hasn't noticed can't write. These writes are a
  refresh's history, the prune, the queue's drain, and, from M14, the
  write path's records.
- **Leader-only work:** the refreshes and the prune, and, from M14, the
  writes. Followers try for the lock at each poll. A lock that a stopping
  leader releases is taken at the next poll, or at once with `LISTEN`.
- **The leader's row:** written under the lock. It holds the replica's name
  (its host name), a random instance ID from its start, when it took over,
  when it was last seen, and when its next refresh is due.
- **Audit:** a takeover is a `service.leader_acquired` event. Its before
  holds the previous leader and when that leader was last seen; its after
  holds the new one.
- **`/status` and metrics:** two new sections, `replica` and `leader`, and
  three new metrics: `nbpdns_leader` (1 on the leader),
  `nbpdns_leader_since_timestamp_seconds` and `nbpdns_leader_changes_total`.
- **SQLite:** `serve` always leads, under its `flock`. There's no lock to
  take, and no leader events.

##### ADR-0048: replicas share their state through the database

- **The change feed:** every replica polls the database every
  `database.poll_interval`, by default 5 seconds. It reads the versions it
  follows: the settings, the groups, the last refresh, the refresh queue and
  the leader. Whatever moved, it reloads. This generalizes M08's settings
  watcher.
- **`LISTEN` and `NOTIFY`:** on PostgreSQL, every change sends `NOTIFY
  nbpdns` in its own transaction. With `database.listen: true`, a replica
  listens on a connection of its own, and polls at once when notified.
  Polling is the default (`false`), as the user chose, and the docs
  recommend turning on `LISTEN` where no transaction-mode pooler sits in
  between. Polling goes on either way, so a lost notification costs at most
  one interval.
- **Served from the database:** every replica's API and `/status` show what
  the database holds:
  - each group's last report and the last refreshes;
  - NetBox's zones, the API's records, which the leader now writes zone by
    zone, only when a zone's digest changes. This lifts M08's "NetBox's
    records aren't persisted": a restarted replica also serves them at once.
- **Commit before serving:** the leader serves a refresh once it's
  committed. If it can't be written, no replica serves it, and it's counted
  as a write failure, as today. This applies to SQLite as well, so that both
  databases follow one rule. Followers reload when the last refresh moves.
- **Status:** `/status` has two kinds of facts, and the API spec says which
  field is which.
  - **The deployment's:** the last and next refresh, the groups, NetBox,
    the webhooks' queue and their last refresh, and the leader. They come
    from the database, and are the same on every replica.
  - **The replica's own:** its build, uptime, readiness, liveness, tracing,
    write failures, and the refreshes it ran.
- **Metrics:** every replica sets the drift gauges from what it loads.
  Counters and histograms count each replica's own work. The docs show how
  to aggregate across replicas with `max by`.
- **Webhooks on any replica:** every replica, on either database, writes a
  webhook's zones to a refresh queue in the database. The leader gathers the
  queue, with ADR-0036's delay and bounds, and deletes the rows its refresh
  covered once that refresh is recorded. A webhook to any replica is
  refreshed, and a queued one survives a restart or a takeover. This lifts
  M07's "a webhook reaches one replica". ADR-0036 stands otherwise.
- **Readiness:** a follower is ready once it has loaded the reports, as a
  restored `serve` is.
- **One config file:** replicas share one config file. Each one mirrors the
  file's groups at its start, as now, so the last one to start wins.

##### Requirements and questions

- REQ-032, REQ-033 and REQ-054 are served. No new REQ is needed.
- The brief gets a dated section, "Answers, 2026-10-18 (M09 design)", with
  three answers:
  - `sslmode` defaults to libpq's `prefer`;
  - the password comes from the URL as a secret, or from a `passfile`;
  - the runner's memory values are deployed.
- The milestone file's "Open for its design" section is marked answered.

#### Design

##### Code

- `internal/store`:
  - `Open` picks the dialect from the URL. The `querier` interface covers
    both dialects, with an adapter for PostgreSQL.
  - The advisory write lock goes in `InTx`. The name sorts go in the list
    methods.
  - New: `leader.go` (the lock, the heartbeat, the fence), `feed.go` (the
    poll query, `NOTIFY` and `LISTEN`) and `queue.go`.
  - New migrations, in item order: `00006_leader.sql`,
    `00007_netbox_zones.sql` and `00008_refresh_queue.sql`.
- `internal/config`:
  - `database.url` accepts `postgres://` URLs, and is a secret with a
    `_FILE` variant.
  - Two new keys: `database.poll_interval` and `database.listen`.
- `internal/cli/serve.go`:
  - The `flock` stays for SQLite. On PostgreSQL, `serve` runs the election
    instead.
  - `prune`, and `service.Run`'s refreshes, run only while this replica
    leads. `watcher` becomes the change feed's consumer.
- `internal/service`:
  - `Restore` becomes a reload, which runs at start and whenever the last
    refresh moves.
  - A refresh is published after its commit.
  - `Notify` writes to the queue, and `Run` gathers the queue.
- `internal/api` and `api/openapi.yaml`:
  - `/status` gains `replica`, `leader`, and the database's `backend`,
    `url` and `tls`, all additive.
  - The spec's descriptions say which facts are the deployment's.
- `internal/metrics`: the three leader metrics. `internal/audit`:
  `service.leader_acquired`.

##### Tests

- The store's tests run on both databases, with a backend list that the
  `integration` build tag extends with PostgreSQL. Each test gets a fresh
  database.
- Contract tests on `/status` cover the new fields, on a leader and on a
  follower.
- **The takeover tests**, in the integration suite:
  - **Setup:** three replicas run as child processes, the test binary
    running itself as `nbpdns serve`. They share a fresh lab database, with
    a 1-second poll.
  - **Cases:**
    1. Exactly one replica refreshes.
    2. A leader **stopped** with SIGTERM is replaced within 5 seconds.
    3. A leader **killed** with SIGKILL is replaced within 5 seconds.
    4. A leader **cut off** by a TCP proxy in the test, which stops
       forwarding but keeps its sockets open, logs its step-down before
       another replica takes over. The takeover comes within 40 seconds.
    5. A webhook sent to a follower is refreshed by the leader, and every
       replica serves the result.
    6. A setting changed with the CLI reaches every replica.
    7. Cases 2 and 5 also pass with `database.listen` on.
- **Memory:** the lab's existing PostgreSQL serves every test, and the
  replicas are small processes. ITEM-0100's 2Gi request leaves room. The
  first pipeline's peak is recorded in ITEM-0110.

##### Docs

- **How-to:**
  - New: "Run nbpdns on PostgreSQL": the role and database, the URL, TLS and
    client certificates, `_FILE` and `passfile`, and poolers.
  - New: "Run several replicas of nbpdns": a Kubernetes and a Compose
    example, `database.listen`, and Prometheus aggregation.
  - Updated: backing up (`pg_dump`), monitoring drift (`max by`), and
    refreshing drift as NetBox changes (any replica).
- **Explanation:** "How nbpdns stores its state" gains PostgreSQL and the
  shared state. A new page, "How nbpdns replicas choose a leader", covers
  the timings and the fence.
- **Reference:** configuration, metrics and audit events, regenerated;
  supported versions (PostgreSQL 18); service endpoints.
- The CHANGELOG, each item in its own change.

#### Items and phases

| Phase | Items |
|---|---|
| M9a Store | The store on PostgreSQL: pgx, sqlc's PostgreSQL package, serialized writes, tests on both, the lab's database |
| | `database.url` for PostgreSQL: TLS, client certificates, the URL as a secret, `serve` and the CLI on it, `/status`'s database |
| M9b Leader | Leader election by a session advisory lock: bounds, heartbeat and step-down, the fence, leader-only work, `/status`, metrics and audit |
| M9c Replicas | The change feed: polling, and `LISTEN` and `NOTIFY` |
| | Every replica serves the API and status from the database, with NetBox's zones stored |
| | Webhooks on any replica, through a queue in the database |
| M9d Testing and docs | The takeover tests: stopped, killed and cut-off leaders |
| | The M09 docs: several replicas on PostgreSQL, and how the leader is chosen |

- **PostgreSQL first:** after M9a, PostgreSQL is usable as a single
  instance. Until M9b, `serve` on PostgreSQL holds the advisory lock, and
  refuses to start while another `serve` holds it, as the `flock` does.
- **Recording this design** is the item "Design M09". It covers:
  - this plan, verbatim, under **Approved design** in M09's file;
  - ADR-0046 to ADR-0048, accepted;
  - the brief's dated answers;
  - the eight items;
  - M09 `in-progress` from 2026-10-18, and GitLab's milestone 11 in step;
  - `make project`, and one commit on `m09-postgresql-and-ha`.

#### Acceptance criteria

- [ ] ADR-0046 to ADR-0048 are accepted, and the brief records the answers of
  2026-10-18.
- [ ] The store runs on PostgreSQL 18 from the same migrations and queries.
  Its tests pass on both databases, and `generate-check` covers both
  packages.
- [ ] `serve` and the CLI take a `postgres://` URL in standard form, with TLS
  and client certificates as libpq has them. The URL can come from
  `NBPDNS_DATABASE_URL_FILE`, or its password from a `passfile`. Its
  password is never logged or shown.
- [ ] With several replicas on one database, exactly one refreshes and
  prunes.
- [ ] When the leader stops or is killed, another replica takes over within
  seconds. When it's cut off, another takes over within about 30 seconds,
  and the old one stops leading first.
- [ ] Every replica serves the same API answers and deployment status from
  the database: within the poll interval, or at once with `database.listen`.
- [ ] A webhook sent to any replica is refreshed by the leader, and survives
  a takeover. Settings and groups changed with the CLI reach every replica.
- [ ] `/status` and the metrics say which replica leads, and since when. A
  takeover is an audit event.
- [ ] SQLite behaves as in M08, a single instance, apart from serving
  committed refreshes only.
- [ ] `make check`, `make test-integration` and `make release-check` pass,
  and the pipelines pass within the runner's memory.
- [ ] ITEM-0100 is closed: M09's merge and tag pipelines pass without a
  retry.
- [ ] The docs exist, the references are regenerated, and the CHANGELOG is
  updated.
- [ ] `/code-review high` and `/security-review` have run. The security
  review covers the database's credentials, TLS and the audit trail.
- [ ] The manual verification is recorded. The user has merged through an MR
  with a merge commit, and tagged `v0.9.0`.

#### Verification

- `make check` on every commit; `make test-integration` and
  `make release-check` locally.
- **Manual,** in the lab:
  1. Run `nbpdns db migrate` and `nbpdns db status` on
     `postgres://…/nbpdns?sslmode=disable`, with the URL from
     `NBPDNS_DATABASE_URL_FILE`. Check that the password is in no log and
     not in `/status`.
  2. Start three `serve`s on the database, on three ports. Check that
     `/status` names one leader on all three, with the same last refresh
     and groups, and that `nbpdns_leader` is 1 on one of them only.
  3. `nbpdns settings set drift.interval 2m --reason test`, and see all three
     apply it.
  4. With the `webhooks` profile, change a record in NetBox, with its webhook
     sent to a follower. See the leader refresh the zone, and all three
     serve the change.
  5. Stop the leader with Ctrl-C. Another takes over within seconds, and
     `nbpdns audit list` shows `service.leader_acquired`.
  6. Kill the new leader with `kill -9`. Another takes over within seconds.
  7. Freeze the leader with `kill -STOP`. Within about 30 seconds PostgreSQL
     ends its session, and another takes over. Then `kill -CONT` it, and see
     it find the lock lost, step down without writing, and follow.
  8. Turn on `database.listen` on all three, and repeat step 5. The takeover
     is at once.
  9. Use `sslmode=require` against the lab's PostgreSQL, which has no TLS,
     and see a clear refusal. With the default, `prefer`, see `/status`
     report the connection as unencrypted.
  10. Start a second `serve` on a SQLite file that one already uses, and see
      it refuse, as in M08.

#### Your side

- Nothing before ITEM-0110. The runner's memory values are already deployed.
- After the merge, tag `v0.9.0`.
