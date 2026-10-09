---
title: '0040: Runtime settings and managed resources'
status: proposed # proposed | accepted | rejected | deprecated | superseded by ADR-NNNN
date: 2026-10-09
decision-makers: [jordan]
requirements: [REQ-050]
questions: [Q-012, Q-024, Q-043]
supersedes: # ADR-NNNN this replaces, if any
---

# 0040: Runtime settings and managed resources

## Context and problem statement

nbpdns's configuration comes from defaults, a file, the environment and
flags (ADR-0021). CLAUDE.md adds that runtime settings live in the database,
unless the file or environment pins them, and that bootstrap keys are never
editable from the web. Q-024 asked who owns a setting that both a UI and IaC
manage; its proposed default is a `managed_by` field on each resource. Q-043
says that once groups are in the database, those from the config file are
`managed_by=file`.

Nothing can change state from outside before M10's authentication. On
2026-10-09 the user chose a local CLI as M08's writer, and that a running
`serve` uses a changed setting within seconds.

## Decision drivers

- A setting's value, and where it came from, are always clear.
- Bootstrap keys, such as the database's location, can't be changed where
  they'd break nbpdns.
- No remote writes before M10.
- Every change is audited (ADR-0039).
- No restart for a runtime change.

## Considered options

1. **The writer:** a local CLI; no writer until M10; the API, without
   authentication.
2. **Applying:** within seconds; at restart; on SIGHUP.

## Decision outcome

Chosen: **a local CLI, applied within seconds**, as the user chose.

- **Two kinds of key:** each key in the registry is a bootstrap or a runtime
  key. Bootstrap keys come only from the file, environment and flags:
  `database.*`, `server.*`, `log.format`, `netbox.url`, `netbox.ca_file` and
  `otlp.*`. Runtime keys are those `serve` can apply live: `drift.*`,
  `audit.retention_days`, `log.level`, the NetBox and PowerDNS timeouts, page
  size and concurrency, `netbox.token` and `netbox.webhook_secret`. The
  configuration reference shows each key's kind.
- **Precedence:** defaults < database < file < environment < flags. A runtime
  key set in the file, environment or flags is pinned: the database's value
  is kept but not used, and `nbpdns settings` says so.
- **The CLI:** `nbpdns settings list|get|set|unset`. Each change bumps the
  settings' version, and is audited with the OS user as its actor, and an
  optional `--reason`. `nbpdns config show` names the database as a source.
- **Applied live:** `serve` checks the settings' version every 5 seconds, and
  applies the keys that changed.
- **Managed resources (Q-024, Q-043):** server groups move into the
  database, each with `managed_by`:
  - `file`: mirrored from the config file at each start, audited, read-only
    to the CLI, and removed when the file drops them. Their API keys stay in
    the file.
  - `cli`: added, changed and removed with `nbpdns groups`; their API keys
    are stored encrypted (ADR-0041).
  - a name that both claim stops `serve`, naming the two;
  - `serve` applies group changes within seconds, making clients for new and
    changed groups and dropping removed ones' state and metrics;
  - the API's server groups gain `managed_by`.
- **Q-012:** this app's settings are runtime settings from M08. Per-zone
  PowerDNS metadata comes with the write path, M14; `pdns.conf` stays with
  Ansible.

### Consequences

- Good: settings and groups change without a restart, and every change is
  audited.
- Good: later managers, the API (M10), the UI (M13) and Terraform (M19), add
  `managed_by` values without a new model.
- Bad: anyone who can write the database file can change settings until M10;
  the file's permissions are the guard.
- Bad: live groups mean the service's groups can change under it, which the
  code must guard.

### Confirmation

- Table tests cover the precedence and pinning, and each runtime key applied
  live.
- An integration test changes `drift.interval` and adds a group with the
  CLI, and sees a running `serve` use them within seconds.

## Pros and cons of the options

### No writer until M10

- Good: nothing writes before authentication.
- Bad: the tables and the precedence go untried until M10.

### The API, without authentication

- Good: remote changes.
- Bad: anyone on the network could change nbpdns, against REQ-046 and the
  plan to authenticate first.

### At restart, or on SIGHUP

- Good: simpler.
- Bad: a change waits for a restart, or for a signal that's awkward to send
  to a container.

## More information

- ADR-0021 (configuration), ADR-0024 (server groups in the file),
  ADR-0039 (audit), ADR-0041 (secrets).
- Recorded in M08's design, 2026-10-09, on `plan-m08-m12` (ADR-0037).
  Implemented by ITEM-0090 and ITEM-0091.
