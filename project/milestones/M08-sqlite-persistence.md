---
id: M08
title: SQLite persistence
status: done # planned | in-progress | done
started: 2026-10-10
closed: 2026-10-17
---

# M08: SQLite persistence

Designed ahead, on `plan-m08-m12` (ADR-0037). Its ADRs, ADR-0038 to
ADR-0042, stay proposed until the milestone starts, on its branch,
`m08-sqlite-persistence`, when the design is checked again and the user
accepts them.

## Goal

`nbpdns serve` keeps its state in SQLite, and survives a restart with its last
reports and their history. Every change to nbpdns's configuration, and every
change of a zone's drift, is an audit event in a hash-chained trail. Runtime
settings and server groups live in the database, each with its owner, are
changed from the command line, and take effect in seconds. Secrets stored in
the database are encrypted with a master key.

## Non-goals

- **No PostgreSQL, no HA:** M09 adds the second dialect and leader election.
  M08's SQL is written to work on both.
- **No remote writes:** settings and groups change only through the local
  CLI. The API stays read-only until M10.
- **No audit API, no SIEM:** the trail is read with `nbpdns audit`, locally.
  The API serves it from M10, behind authentication; M17 exports it.
- **No export, import or backup commands:** M18 (Q-026). M08's docs say how to
  copy the SQLite file safely.
- **No per-zone PowerDNS metadata:** the write path, M14 (Q-012).
- **No Vault or OpenBao:** the master key comes from a file or the environment
  (Q-023).
- **NetBox's records aren't persisted:** they're a cache that the first refresh
  after a restart reads again.

## Phases

| Phase | Items |
|---|---|
| M8a Store | ITEM-0086 The SQLite store: driver, migrations, sqlc and the lock |
| M8b Audit | ITEM-0087 The audit core: events, chain, retention and `nbpdns audit` |
| M8c History | ITEM-0088 Drift history and restore at start |
| | ITEM-0089 Drift history in the API and the CLI |
| M8d Settings | ITEM-0090 Runtime settings: key kinds, precedence and live application |
| | ITEM-0091 Server groups in the database, with `managed_by` |
| M8e Secrets | ITEM-0092 Secrets encrypted at rest, and master key rotation |
| M8f Testing and docs | ITEM-0093 The M08 integration tests and the image's volume |
| | ITEM-0094 The M08 docs and CHANGELOG |

ITEM-0085 recorded this design.

## Acceptance criteria

- [x] ADR-0038 to ADR-0042 are accepted, after the design is checked at M08's
  start. REQ-049 to REQ-053 exist, and the questions are answered.
- [x] `serve` keeps its state in SQLite at `database.url`, applies migrations
  at start, and a second `serve` on the same database refuses to start.
- [x] After a restart, `serve` serves its last reports and history at once,
  and says how old they are.
- [x] Each zone's history holds its changes of state and its RRset changes,
  served by the API and `nbpdns drift history`, and pruned after its
  retention.
- [x] Every change of configuration, and every change of a zone's drift, is
  an audit event; `nbpdns audit verify` passes on a good trail, and fails on
  an edited or broken one.
- [x] Runtime settings and groups change through the CLI, are audited with the
  OS user, respect pinning and `managed_by`, and reach a running `serve`
  within seconds.
- [x] Stored secrets are encrypted with the master key, which rotates without
  loss.
- [x] The integration tests and the release check pass, with the image's
  database on a volume.
- [x] The docs exist, the references are regenerated, and the CHANGELOG is
  updated.
- [x] `/code-review high` and `/security-review` have run.
- [ ] The manual verification is recorded, the pipelines pass, and the user
  has merged through an MR with a merge commit, and tagged `v0.8.0`.

## Verification log

Append-only and dated. Record what was run and what was seen.

- 2026-10-10: **The design checked at the start** (ADR-0037), on
  `m08-sqlite-persistence`, after `plan-m08-m12`'s merge and the v0.7.0
  release. The user accepted ADR-0038 to ADR-0042 unchanged.
  - **The driver:** a throwaway module's cold `go test -race` of
    modernc.org/sqlite v1.60.1, in WAL mode, took 29 s on 16 cores, its
    largest process peaking at 0.95 GB; ncruces/go-sqlite3 v0.35.6 took
    18 s and 0.69 GB. The repo's own cold `go test -race` already peaks at
    1.28 GB, so modernc.org/sqlite doesn't raise it. The runner's failure
    on main's pipeline 795 came from two pipelines at once (ITEM-0100).
  - **sqlc:** v1.31.1's linux-amd64 release is a static binary. From one
    goose-style schema and one query file with `sqlc.arg` and
    `sqlc.narg`, it generated both a SQLite and a PostgreSQL package. The
    PostgreSQL engine refused `INSERT OR REPLACE`, `GLOB` and
    `AUTOINCREMENT`, but not the SQLite-only functions `IFNULL` and
    `datetime`. So the test that refuses SQL PostgreSQL wouldn't take is
    sqlc's PostgreSQL engine in `generate-check`, and a test that refuses
    SQLite-only functions.
  - **Portable schemas:** `TEXT`, `BIGINT` and `BOOLEAN` columns only;
    nbpdns assigns its own IDs, since SQLite's rowid alias and
    PostgreSQL's identity columns can't share a migration; hashes are
    stored as hex, and ciphertexts as base64.
- 2026-10-17: **Checks at the close.** On `f1729c5`, `make check`,
  `make build`, `make docs-links` and `make release-check` passed, and
  `make test-integration` passed from Go's test cache, so it isn't counted.
  After ITEM-0107's fixes, on `4232ef6`, `make check` passed: vet, lint,
  the unit tests with `-race`, govulncheck (GO-2026-5932, x/crypto's
  openpgp, in a required module that the code doesn't call), three gitleaks
  scans with no leaks, docs, API and project lint, the generated files up
  to date, and every module tidy. With `GOFLAGS=-count=1`, `make
  test-integration` passed on the lab: internal/cli 116.9 s, internal/lab
  1.1 s, internal/netbox 6.2 s, internal/powerdns 1.3 s, the two tools
  modules skipped. `make release-check` passed, internal/release 8.8 s,
  whose "serve on a volume" runs serve in the image on a named volume and
  on a bind mount owned by 65532.
- 2026-10-17: **`/code-review high`**, on `main...m08-sqlite-persistence`,
  found ten things. Seven are fixed in ITEM-0107: a data race between a
  refresh's `reconfigure` and the watcher's read of the master key; NetBox
  shown as up by a refresh with no groups, which didn't read it; a failed
  reload never tried again; a new group handed a removed one's state for
  reusing its name; the prune and the watcher left running while serve
  closed the database; an audit prune that could delete an event younger
  than its retention, for following an older one; and three readers of a
  secret file. Three are left, with reasons in ITEM-0107: a refresh that a
  group change brought forward is stored as `schedule`, by design, since
  `cause` is a closed enum (ADR-0012); `diffGroups` and `SetGroups` can't
  share code, since the service lacks the connection settings; and the
  user's own commit 3ffda43 has no `Refs` trailer, and stays as pushed.
- 2026-10-17: **`/security-review`**, since M08 touches audit, secrets and
  crypto: no findings at its bar, confidence 8 of 10 or more. It checked the
  envelope encryption (a fresh data key and nonces from crypto/rand,
  AES-256-GCM, each value and data key bound to its place, so a ciphertext
  moved between settings or groups doesn't open), the rotation's one
  transaction, that no secret reaches a log, an audit value, `settings`,
  `groups list`, `config show`, `/status` or the API, that every query is
  a static sqlc query with bound parameters, the chain's canonical form,
  and the database's and lock's modes. Considered and dismissed: someone
  with write access to the database, but not the master key, could point a
  CLI group at another URL, and serve would send its key there, which
  ADR-0041's threat model leaves out (binding the URL into the seal's
  associated data would cover it); the unkeyed chain, which ADR-0039
  accepts until M17's off-host copy; a lock-file symlink, which needs write
  access to the database's directory; and `/status` showing the database's
  path.
- 2026-10-17: **Manual verification**, on the lab, with `bin/nbpdns` built
  at `4232ef6`, a view `m08-verify` of two zones in sync on lab-a, a config
  file with lab-a and no NetBox token, and the database in a scratch
  directory. `nbpdns db migrate`, then `nbpdns settings set netbox.token
  --from-file` with a master key, stored it sealed, in a file of mode 0600.
  1. `serve` read NetBox with the token from the database: both zones in
     sync. Stopped with SIGINT and started again with
     `NBPDNS_NETBOX_URL=http://127.0.0.1:1`, it was ready at once: `/readyz`
     200 before any refresh, `/api/status` `restored: true` as of the last
     refresh, the zones and history served from the database, and its log
     said the reports were 52.6 s old. The refresh then failed for NetBox,
     and every group kept its report.
  2. A PATCH of zone a's www A on PowerDNS drifted it at the next refresh,
     one change; putting it back brought it in sync. The API's history and
     `nbpdns drift history --zone` showed the same three rows, in sync,
     drift, in sync, and `nbpdns drift history` the three refreshes. Each
     change of state was a `zone.drift_changed` event.
  3. `nbpdns settings set drift.interval 1m --reason test` reached serve in
     4.0 s: `/status` showed 60 s, the next refresh a minute after the last
     one started, and `nbpdns audit list` the `setting.set` by
     `os-user:jordan`, with its reason.
  4. `nbpdns groups add lab-b`, on the same primary and view, was compared
     in 5.0 s, `managed_by: cli`, with lab-a's counts; `groups list` showed
     lab-a's owner as the config file. `groups remove lab-b` took it off
     the API, and its metric series went, within 5 s. Both were `group.*`
     events by the OS user, the API key `[redacted]`.
  5. With serve stopped, `nbpdns secrets rotate-master-key --new-key-file`
     re-wrapped the token, as a `master_key.rotated` event. The old key was
     refused, naming both key IDs; with the new one, serve started and read
     NetBox. Its output said "Re-wrapped 1 secrets", fixed in ITEM-0108.
  6. A copy made with `sqlite3`'s `.backup` while serve ran passed `nbpdns
     audit verify`, 13 events. With one event's reason edited, verify failed
     at it, "its contents don't match its hash"; with one deleted, it failed
     at the next, "the events from 6 to 6 are missing".
  7. A second `serve` on the same database exited 1: "another nbpdns holds
     the database's lock", naming the first one's host and PID.
  The three runs logged 129 lines, with no errors, and neither secret. The
  zones, view and nameserver were removed from the lab after.
- 2026-10-17: **Acceptance criteria**, and what each tick rests on:
  - The store, migrations and lock: `TestOpenCreatesTheDatabase`,
    `TestMigrate`, `TestMigrateFromEachVersion`, `TestLock`,
    `TestServeOneInstancePerDatabase`, `TestServeRestarts`, and manual
    step 7.
  - Restore at start: `TestRestoreDrift`, `TestServeRestarts`, and manual
    step 1.
  - Zone history: `TestZoneHistory`, `TestPruneDriftHistory`,
    `TestReadHistory`, `TestServeRestarts`, and manual step 2.
  - The audit trail: `TestVerifyAudit` (an edited event, its hash made
    again, a deleted event, the first deleted, a forged anchor),
    `TestPruneAudit`, `TestPruneAuditKeepsAYoungerEvent`,
    `TestServeRestarts`, and manual steps 2 to 6.
  - Settings and groups: `TestServeAppliesSettings` (pinned by the
    environment), `TestServeRetriesAFailedReload`,
    `TestGroupsAndTheConfigFile` (a file group refused), `TestCLIGroups`,
    `TestServeAppliesCLIChanges`, and manual steps 3 and 4.
  - Secrets: `TestSealAndOpen`, `TestOpenRefuses`, `TestRewrap`,
    `TestRotateMasterKey`, `TestSecretSettings`, and manual step 5.
  - The integration tests and the release check: the checks above.
  - The docs: the explanation "How nbpdns stores its state"; the how-tos on
    settings, groups, the master key, the audit trail, drift history and
    backing up the database; "Install nbpdns" and "Run nbpdns in a
    container" with the volume; the references regenerated, checked by
    `generate-check`; and CHANGELOG's `[0.8.0]` section, which
    `releasenotes -tag v0.8.0` prints.
  - The reviews: the two entries above.
  ITEM-0100, whose last criterion is that the merge's and the tag's
  pipelines pass without a retry, moved to M09, as the user chose: a done
  milestone can't hold an item in progress.

## Approved design

The plan approved on 2026-10-09, copied verbatim. Its headings are demoted two
levels to nest under this section; the text is unchanged. It's a snapshot.
Where it disagrees with an ADR or `CLAUDE.md`, they win.

### M08: SQLite persistence

#### Context

`nbpdns serve` keeps everything in memory: each group's last report, the
refresh history, the webhooks' state. A restart forgets it all, and the first
refresh rebuilds it a minute later. Nothing is audited yet, configuration
lives only in files, environment and flags, and no secret is stored. M08 gives
nbpdns a database, embedded SQLite (ADR-0009), and builds on it what the later
milestones need: an audit trail, runtime settings with an owner for each, the
server groups as managed resources, secrets encrypted at rest, and a drift
history.

This is the first milestone designed ahead, on `plan-m08-m12` (ADR-0037). Its
ADRs stay `proposed` until M08 starts, when the design is checked again and
the user accepts them.

Decided with the user in the M08 design session, 2026-10-09:
- **Scope:** as stubbed, all six parts, not thinned.
- **Query layer:** sqlc, on goose's forward-only SQL migrations and the
  pure-Go modernc.org/sqlite driver.
- **Audit (Q-035):** every state change is an event, hash-chained from the
  first one, with a configurable retention.
- **Compliance (Q-036):** built to support the Essential Eight and the ISM,
  ISO 27001, and SOC 2 or PCI DSS. The strictest defaults win: audit events
  kept for seven years, the ISM's minimum for event logs, and ASD-approved
  cryptography.
- **The writer before M10:** a local CLI that writes the database directly,
  each change audited with the OS user as its actor.
- **Applying settings:** a running `serve` uses a changed runtime setting
  within seconds.
- **Drift history:** each zone's changes of state, and its RRset changes at
  each refresh, not only the last state.
- **The database:** required for `serve`, at `database.url`, by default
  `sqlite:///var/lib/nbpdns/nbpdns.db`.

#### Goal

`nbpdns serve` keeps its state in SQLite, and survives a restart with its last
reports and their history. Every change to nbpdns's configuration, and every
change of a zone's drift, is an audit event in a hash-chained trail. Runtime
settings and server groups live in the database, each with its owner, are
changed from the command line, and take effect in seconds. Secrets stored in
the database are encrypted with a master key.

#### Non-goals

- **No PostgreSQL, no HA:** M09 adds the second dialect and leader election.
  M08's SQL is written to work on both.
- **No remote writes:** settings and groups change only through the local
  CLI. The API stays read-only until M10.
- **No audit API, no SIEM:** the trail is read with `nbpdns audit`, locally.
  The API serves it from M10, behind authentication; M16 exports it.
- **No export, import or backup commands:** M17 (Q-026). M08's docs say how to
  copy the SQLite file safely.
- **No per-zone PowerDNS metadata:** the write path, M13 (Q-012).
- **No Vault or OpenBao:** the master key comes from a file or the environment
  (Q-023).
- **NetBox's records aren't persisted:** they're a cache that the first refresh
  after a restart reads again.

#### Decisions

##### ADR-0038: an embedded SQLite store, with goose and sqlc (proposed)

- **Driver:** modernc.org/sqlite, pure Go, so builds stay static with CGO off
  (ADR-0009). WAL mode, and a busy timeout, so that the CLI can write while
  `serve` runs.
- **Migrations:** goose, with SQL files embedded in the binary, forward-only
  (Q-026). `serve` applies pending migrations at start, and logs them;
  `nbpdns db status` and `nbpdns db migrate` show and apply them. The SQL
  stays portable to PostgreSQL, and a lint test refuses SQLite-only syntax.
- **Queries:** sqlc, generating type-checked Go from `internal/store/*.sql`
  into `internal/store/sqlite`, with database/sql. Named parameters
  (`sqlc.arg`), so that M09 generates its PostgreSQL package from the same
  queries. `make generate` runs it; `generate-check` keeps it current. sqlc is
  a pinned release binary in `tools/tools.mk`, since it needs CGO to build.
- **Location:** `database.url`, a bootstrap key, defaults to
  `sqlite:///var/lib/nbpdns/nbpdns.db`. `serve` fails clearly if it can't
  open, create or lock it. The one-shot commands, such as `nbpdns drift`,
  need no database.
- **The image:** ko adds only the binary to distroless, so the image can't
  make `/var/lib/nbpdns` or declare it a volume. Deployments mount it:
  Kubernetes with `fsGroup: 65532`; Docker with a bind mount owned by 65532.
  The release check runs the image with such a mount, and writes the
  database.
- **One instance:** `serve` takes an exclusive `flock` on `<database>.lock`,
  and refuses to start while another `serve` holds it, naming its host and
  PID (ADR-0009). The kernel releases it if `serve` dies. The CLI's writes
  don't take it.

##### ADR-0039: a hash-chained audit trail (proposed)

- **Events:** actor (type and ID: `os-user:jordan`, `system`, `file`),
  action, target (type and ID), before and after (JSON), reason, `trace_id`,
  `request_id`, source IP, outcome, and time.
- **Declared once:** each action is declared in `internal/audit`'s registry,
  with its target type and meaning, and `docs/reference/audit-events.md` is
  generated from it.
- **Chained:** each event stores the SHA-384 of its canonical JSON and the
  hash before it. SHA-384 is an ASD-approved algorithm. `nbpdns audit verify`
  walks the chain, and fails on a gap or an edit.
- **Atomic:** a change and its event are written in one transaction, so
  neither exists without the other.
- **Retention:** `audit.retention_days`, default 2557, seven years. A daily
  prune deletes older events, keeps the hash of the last one it deleted as
  the chain's anchor, and is itself an event.
- **Reading:** `nbpdns audit list`, filtered by time, action, actor and
  target, as text or JSON.
- **M08's events:** settings set and unset; groups created, changed and
  removed, by the CLI or by the config file's mirror; master key rotated;
  audit pruned; the service started, with its configuration's fingerprint,
  and stopped; and a zone's drift state changed (ADR-0008).

##### ADR-0040: runtime settings and managed resources (proposed)

- **Two kinds of key:** each key in the registry is a **bootstrap** key or a
  **runtime** key. Bootstrap keys stay in the file, environment and flags:
  `database.*`, `server.*`, `log.format`, `netbox.url`, `netbox.ca_file` and
  `otlp.*`. Runtime keys are those that `serve` can apply live: `drift.*`,
  `audit.retention_days`, `log.level`, the NetBox and PowerDNS timeouts,
  page size and concurrency, `netbox.token` and `netbox.webhook_secret`.
  The configuration reference shows which kind each key is.
- **Precedence:** defaults < database < file < environment < flags. A runtime
  key set in the file, environment or flags is **pinned**: the database's
  value is kept but not used, and `nbpdns settings` says so.
- **The CLI:** `nbpdns settings list|get|set|unset`. Each change bumps the
  settings' version, and is audited with the OS user as its actor and an
  optional `--reason`. `nbpdns config show` names the database as a source.
- **Applied live:** `serve` checks the settings' version every 5 seconds, and
  applies the changed keys: the schedule and delays at once, `log.level`
  through a level variable, and new clients for a new token or timeout.
- **Groups are managed resources** (Q-024, Q-043), each with `managed_by`:
  - `file`: the config file's groups, mirrored into the database at each
    start, audited, and read-only to the CLI. A group gone from the file is
    removed. The API key stays in the file, not the database.
  - `cli`: added with `nbpdns groups add`, changed with `set`, removed with
    `remove`. Its API key is stored, encrypted.
  - A name that both claim stops `serve`, with an error naming the two.
  - `serve` applies group changes within seconds too: it makes clients for
    new and changed groups, and drops the state and metrics of removed ones.
  - The API's server groups gain `managed_by`.

##### ADR-0041: secrets encrypted at rest (proposed)

- **Envelope encryption (Q-023):** each stored secret has its own data key;
  the value is sealed with AES-256-GCM, and the data key is wrapped with the
  master key, AES-256-GCM too, both ASD-approved. The master key's ID is
  stored with each wrapped key.
- **The master key:** `database.master_key`, a bootstrap secret with a
  `_FILE` form: 32 random bytes, base64. It's needed only once a secret is
  stored in the database. Without it, storing one fails, and `serve` refuses
  to start if the database holds secrets it can't open.
- **What's encrypted:** secret runtime settings, such as `netbox.token`, and
  the API keys of CLI-managed groups. A file-managed group's key stays in the
  file.
- **Rotation:** `nbpdns secrets rotate-master-key --new-key-file` re-wraps
  every data key in one transaction, and is audited.

##### ADR-0042: drift history (proposed)

- **Refreshes:** each refresh, full or of zones, its trigger, times, outcome,
  error, zones, and NetBox's request IDs, and each group's result in it.
- **Zone history:** a zone's row is written when its state, or the digest of
  its RRset changes, differs from its last row, with the changes themselves,
  the serials, the policy and the refresh. A zone that leaves the report gets
  a last row. So what drifted, exactly, at any time can be read back, while a
  refresh that changes nothing writes nothing per zone.
- **Restore:** each group's last report is kept whole, replaced at each
  refresh, and read back at start, with the last refresh's outcome. `serve`
  is ready once it has restored, or, with nothing to restore, once the first
  refresh has finished; `/status` says which, and how old the report is. The
  first refresh starts at once either way.
- **Retention:** `drift.history_retention_days`, default 400, more than PCI
  DSS's year. A zone's change of state is also an audit event, kept with the
  audit trail's retention.
- **Reading:** `GET /api/server-groups/{group}/zones/{zone}/history` and
  `GET /api/server-groups/{group}/refreshes`, paged with cursors, and
  `nbpdns drift history`. The API's version becomes 1.2.0.

##### Requirements and questions

- **REQ-049:** every change to nbpdns's configuration, and every change of a
  zone's drift, is an audit event in a hash-chained trail, kept for a
  configurable time, by default seven years.
- **REQ-050:** runtime settings and server groups live in the database, each
  with its owner, and take effect without a restart.
- **REQ-051:** secrets stored in the database are encrypted at rest with
  ASD-approved algorithms.
- **REQ-052:** nbpdns keeps each zone's drift history, with its changes, for a
  configurable time.
- **REQ-053:** nbpdns is built to support the Essential Eight and the ISM, ISO
  27001, and SOC 2 or PCI DSS; where they differ, the strictest default wins.
- **Answered:** Q-035 and Q-036, as above; Q-023, Q-024 and Q-026's
  migrations part, with their proposed defaults; Q-012's settings part. Q-012's
  per-zone metadata part stays open, for M13, and Q-026's export and backup
  part, for M17.

#### Design

##### Code

- **`internal/store`** (new): opening the database (`database.url`, WAL, busy
  timeout), the lock, the migrations (`migrations/*.sql`, embedded), the
  queries, and the generated `sqlite` package. Transactions wrap each change
  with its audit event.
- **`internal/audit`** (new): the event model, the action registry, the
  canonical encoding and chain, append, list, verify and prune.
- **`internal/secrets`** (new): envelope encryption, the master key, rotation.
- **`internal/config`:** `Key.Runtime`, the database as a source between the
  defaults and the file, pinning, and the new keys: `database.url`,
  `database.master_key`, `audit.retention_days`,
  `drift.history_retention_days`.
- **`internal/service`:** restore at start; writing each refresh, report and
  zone history; the settings watcher, and live application of settings and
  groups, with the groups held behind a lock rather than fixed at start; the
  daily prune; the status's `database` section.
- **`internal/cli`:** `db status|migrate`, `settings`, `groups`, `audit
  list|verify`, `secrets rotate-master-key`, `drift history`; `serve` opens
  the store, mirrors the file's groups, and starts the watcher.
- **`internal/api`:** the history and refreshes operations, and `managed_by`.
- **Metrics:** `nbpdns_audit_events_total{action}`,
  `nbpdns_settings_reloads_total{outcome}`, and
  `nbpdns_db_write_failures_total{op}`.
- **Failures:** a refresh whose history can't be written is still served from
  memory, and counted, logged and shown on `/status`. A CLI change and its
  event are one transaction, so a failed write changes nothing.

##### Tests

- **Unit, table-driven:** the precedence and pinning; the chain, with an
  edited event, a deleted one and a pruned range; envelope encryption, a
  wrong key and rotation; the zone history's rows across refreshes; restore;
  the live application of each runtime key; the groups' mirror, conflicts and
  removal; the migrations on an empty and a migrated database; and a test
  that refuses SQL that PostgreSQL wouldn't take.
- **Integration (CI), on the lab:** `serve` restarts and still serves its last
  reports and history; a second `serve` on the same database refuses to
  start; `nbpdns settings set drift.interval` and `nbpdns groups add` change a
  running `serve` within seconds; `nbpdns audit verify` passes, and fails
  after the database is edited.
- **Release check:** the image runs with a volume owned by 65532 at
  `/var/lib/nbpdns`, and writes its database there.

##### Docs

| Section | Pages |
|---|---|
| Explanation | "How nbpdns stores its state": the database, migrations, the lock, settings and their precedence, managed resources, encryption, the audit chain, and history. |
| How-to | "Change nbpdns's settings while it runs"; "Manage server groups from the command line"; "Protect stored secrets with a master key", with rotation; "Check the audit trail"; "Read a zone's drift history". "Run nbpdns in a container" and "Install nbpdns" gain the database's volume. |
| Reference | The configuration (with each key's kind), the audit events (new, generated), the command line, the API, the metrics, and "Service endpoints". |

The CHANGELOG gets lines under Unreleased.

#### Items and phases

| Phase | Items |
|---|---|
| M8a Store | The SQLite store: driver, migrations, sqlc, `database.url`, the lock |
| M8b Audit | The audit core: events, registry, chain, retention, `nbpdns audit` |
| M8c History | Drift history and restore at start |
| | Drift history in the API and the CLI |
| M8d Settings | Runtime settings: the key kinds, precedence, `nbpdns settings`, live application |
| | Server groups in the database: `managed_by`, the file's mirror, `nbpdns groups`, live application |
| M8e Secrets | Secrets encrypted at rest, and the master key's rotation |
| M8f Testing and docs | The integration tests and the image's volume |
| | The docs and the CHANGELOG |

Recording this design is ITEM "Design M08", on `plan-m08-m12`: this plan
verbatim under **Approved design** in M08's file, ADR-0038 to ADR-0042
(proposed), REQ-049 to REQ-053, the answered questions, the brief's dated
answers, and the items. M08 stays `planned` until its branch,
`m08-sqlite-persistence`, starts after `plan-m08-m12` is merged.

#### Acceptance criteria

- [ ] ADR-0038 to ADR-0042 are accepted, after the design is checked at M08's
  start. REQ-049 to REQ-053 exist, and the questions are answered.
- [ ] `serve` keeps its state in SQLite at `database.url`, applies migrations
  at start, and a second `serve` on the same database refuses to start.
- [ ] After a restart, `serve` serves its last reports and history at once,
  and says how old they are.
- [ ] Each zone's history holds its changes of state and its RRset changes,
  served by the API and `nbpdns drift history`, and pruned after its
  retention.
- [ ] Every change of configuration, and every change of a zone's drift, is
  an audit event; `nbpdns audit verify` passes on a good trail, and fails on
  an edited or broken one.
- [ ] Runtime settings and groups change through the CLI, are audited with the
  OS user, respect pinning and `managed_by`, and reach a running `serve`
  within seconds.
- [ ] Stored secrets are encrypted with the master key, which rotates without
  loss.
- [ ] The integration tests and the release check pass, with the image's
  database on a volume.
- [ ] The docs exist, the references are regenerated, and the CHANGELOG is
  updated.
- [ ] `/code-review high` and `/security-review` have run.
- [ ] The manual verification is recorded, the pipelines pass, and the user
  has merged through an MR with a merge commit, and tagged `v0.8.0`.

#### Verification

- `make check` on every commit; `make test-integration` and
  `make release-check` locally.
- **Manual:**
  1. Run `serve` on the lab, let it refresh, restart it, and see `/status` and
     `/api` serve the last reports at once.
  2. Drift a zone in PowerDNS, fix it, and read its history through the API
     and `nbpdns drift history`.
  3. `nbpdns settings set drift.interval 1m --reason test`, and see `serve`
     use it within seconds, and the event in `nbpdns audit list`.
  4. `nbpdns groups add` a second group, see it compared, then remove it.
  5. Store `netbox.token` in the database with a master key, rotate the key,
     restart, and see NetBox still read.
  6. Edit an audit row with `sqlite3`, and see `nbpdns audit verify` fail.
  7. Start a second `serve` on the same file, and see it refuse.

#### Your side

- Nothing until M08 starts. Then, for the manual steps, a master key: 32
  random bytes, base64, such as `openssl rand -base64 32` prints.
- After the merge, tag `v0.8.0`.
