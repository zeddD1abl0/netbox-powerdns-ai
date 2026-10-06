---
id: M03
title: Drift report
status: in-progress # planned | in-progress | done
started: 2026-10-06
closed:
---

# M03: Drift report

## Goal

`nbpdns drift` compares the zones NetBox assigns to each server group with
what the group's primary serves, and reports every difference as a table or
JSON, with exit codes that scripts can act on. Read-only.

## Non-goals

- No writes, so `enforce` reports like `report` until M12.
- No secondaries: only primaries are read, as in M02. Secondaries are checked
  with DNS queries in M13.
- No state, history, schedule or alerts: those need the service (M04) and the
  database (M07). Q-027's "keep the last-known state" applies there.
- No drift metrics (M04), and no API (M05).

## Phases

| Phase | Items |
|---|---|
| M3a Policy | ITEM-0042 Drift policies in the server groups' config |
| M3b Comparison | ITEM-0043 Compare zones and report drift (ADR-0027, REQ-043), with the scale benchmark |
| M3c Command | ITEM-0044 `nbpdns drift`: reading both sides, failures, exit codes, table and JSON |
| M3d Lab and docs | ITEM-0045 Drift fixture and integration tests; ITEM-0046 Drift docs and CHANGELOG |

## Acceptance criteria

- [ ] ADR-0027 is accepted. Q-017 and Q-027 are answered, and REQ-043
  exists.
- [ ] Drift policies load from the config file, with strict validation, are
  shown by `config show`, and are documented in the reference.
- [ ] The comparison handles every case in ADR-0027, covered by table tests.
  The benchmark compares 1,000 zones and 100,000 records in under a second,
  and its figures are recorded.
- [ ] `nbpdns drift` reports as a table and as JSON, with `--group` and
  `--zone`, and exits 0, 3, 1 or 2 as ADR-0027 says.
- [ ] Integration tests cover every case of the drift fixture against the
  lab's NetBox 4.7 and PowerDNS 5.1, and a group that can't be read. The lab
  gains no containers.
- [ ] The docs pages above exist, the generated references are current, and
  the CHANGELOG is updated.
- [ ] `/code-review high` has run. `/security-review` runs if anything touches
  secrets.
- [ ] The manual verification is recorded. The GitLab and GitHub pipelines
  pass, and the user has merged through an MR with a merge commit.

## Verification log

Append-only and dated. Record what was run and what was seen.

## Approved design

The plan approved on 2026-10-06, copied verbatim. Its headings are demoted two
levels to nest under this section; the text is unchanged. It's a snapshot.
Where it disagrees with an ADR or `CLAUDE.md`, they win.

### M03: Drift report

#### Context

M01 reads NetBox's DNS data, and M02 reads each PowerDNS server group's
primary, both into one normalized RRset model (ADR-0023, ADR-0025), with
NetBox's zones assigned to groups by view (ADR-0026). M03 compares the two
and reports every difference: the first useful answer to "does PowerDNS
serve what NetBox says?". It's read-only, runs once per command, keeps no
state, and adds nothing to the lab, whose memory the CI runners can't spare.

Decided with the user in the M03 design session, 2026-10-06:
- **Drift policy (ADR-0008's open point):** set in nbpdns's config file, per
  server group, with overrides per zone. A zone with no policy is `report`.
- **Zones on a primary that NetBox doesn't assign to its group** are listed as
  unmanaged, and don't count as drift. Brownfield import (M14) adopts them.
- **SOA:** compared on every field but the serial, which PowerDNS rewrites
  itself (SOA-EDIT-API).
- **Scale (Q-017):** designed and tested for 1,000 zones and 100,000 records
  for now, raised when a deployment needs more.

#### Goal

`nbpdns drift` compares the zones NetBox assigns to each server group with
what the group's primary serves, and reports every difference as a table or
JSON, with exit codes that scripts can act on. Read-only.

#### Non-goals

- No writes, so `enforce` reports like `report` until M12.
- No secondaries: only primaries are read, as in M02. Secondaries are checked
  with DNS queries in M13.
- No state, history, schedule or alerts: those need the service (M04) and the
  database (M07). Q-027's "keep the last-known state" applies there.
- No drift metrics (M04), and no API (M05).

#### Decisions

- **ADR-0027, the drift report:**
  - **What's compared:** what each side serves. NetBox's active records,
    against the primary's records that aren't disabled. A record that
    neither side serves (inactive in NetBox, disabled in PowerDNS) isn't
    compared.
  - **Zones:** an active NetBox zone in one of the group's views is expected
    on the primary. Missing there, it's drift (`missing`). A NetBox zone that
    isn't active, such as parked, is expected absent; on the primary, it's
    drift (`inactive_in_netbox`). A zone on the primary that NetBox doesn't
    assign to the group is `unmanaged`: listed, not drift.
  - **RRsets:** keyed by owner name and type. Only in NetBox: `missing`.
    Only on the primary: `extra`. In both: `changed` if the values or the TTL
    differ, with each side's TTL and values.
  - **SOA:** compared on every field but the serial; each side's serial is
    shown.
  - **Problems** normalization found, on either side, are listed beside the
    drift, and aren't drift.
  - **Policies:** `ignore` skips a zone, which is listed as ignored. `report`
    and `enforce` are compared and reported; `enforce` is marked as acting
    from M12.
  - **Failures (Q-027 for M03):** if NetBox can't be read, nothing can be
    compared, and the command fails. If a group's primary can't be read,
    that group is marked failed, with the reason, and the others are still
    compared. A failed group makes the run fail, since its report is
    incomplete.
  - **Exit codes:** 0, everything compared with no drift; 3, everything
    compared and drift found; 1, something couldn't be read, which wins over
    drift; 2, a usage error, as before.
  - **Q-039:** the comparison is a pure function over the shared model, and
    reading goes through two small interfaces defined by `internal/drift`:
    one for NetBox, one for a group's primary.
  - **Q-017:** 1,000 zones and 100,000 records per run, with time and memory
    measured.
- **Drift policy in the config file:** each group gets `drift_policy`
  (`report` by default) and `zone_policies`, a map from zone name to policy.
  Policy names are `enforce`, `report` and `ignore` (REQ-031). A policy for a
  zone the group doesn't serve is a warning, not an error, since NetBox can
  change without the config.
- **Requirements:** REQ-043 "nbpdns is designed and tested for 1,000 zones and
  100,000 records per run". Q-017 and Q-027 move to Answered. ADR-0008's open
  point is answered by ADR-0027.

#### Design

##### Configuration (`internal/config/groups.go`)

```yaml
powerdns:
  groups:
    - name: site-a
      views: [_default_]
      drift_policy: report          # the group's default; report if unset
      zone_policies:                # by zone name, as in NetBox
        legacy.example.com: ignore
        example.com: enforce
      primary: { ... }
```

Two new fields in `GroupFields`, with strict decoding and validation:
- a policy must be one of the three;
- zone names go through `netbox.ZoneName`'s rules (ASCII, no final dot
  needed);
- the same name listed twice is an error.

`config show` lists them, such as `powerdns.groups.site-a.zone_policies` as
`example.com=enforce,legacy.example.com=ignore`. The generated reference
documents them.

##### The comparison (`internal/drift`)

- `Compare(group, netboxZones, primaryZones []dns.Zone, policies) GroupReport`
  is pure, with no I/O, and is table-tested for every case in ADR-0027.
- A report holds:
  - each group, with its status (`ok` or `failed`, and why);
  - its zones, each with a state: `in_sync`, `drift`, `missing`,
    `inactive_in_netbox` or `ignored`, plus its policy and its RRset
    changes;
  - its unmanaged zones;
  - the problems found.
  It also counts each state.
- `Run(ctx, NetBoxSource, []GroupSource)` reads NetBox once, for the union of
  every group's views, then each primary, and compares them. Each group runs
  in turn; reads within a group use the configured concurrency.
- **Adapters** in `internal/cli` wrap `netbox.Client`, with `Zones` by views
  and `ReadZones`, and `powerdns.Client`, with `Zones` and `ReadZones`.
- **The scale test:** a benchmark generates 1,000 zones and 100,000 records on
  each side, with 1% drift, and records time and allocations. It must compare
  in under a second.

##### The command (`nbpdns drift`)

| Flag | Does |
|---|---|
| `--group G` | Only compare group G |
| `--zone Z` | Only compare zone Z, in every group that serves it. It reads only that zone, on both sides. |
| `--output table\|json` | As the other commands |

- **The table** prints a summary line per group, then a row per change for
  each zone that drifted: group, zone, name, type, change, NetBox's
  TTL/values, and the primary's. Unmanaged zones, ignored zones and problems
  follow, each under a heading of its own.
- **JSON** is the whole report.
- Exit codes as in ADR-0027; `cli.go` gains `exitDrift = 3`, and the
  command-line reference lists every command's exit codes.

##### Lab and tests

- **No new containers.** A drift fixture (`internal/lab`) creates one view
  and a set of zones in NetBox, and the matching zones on lab-a's primary,
  with one of each case:
  - a zone in sync;
  - a missing RRset;
  - an extra RRset;
  - a changed value;
  - a changed TTL;
  - an SOA that differs only in its serial (in sync);
  - an SOA that differs in its contact;
  - a zone missing on the primary;
  - an unmanaged zone on the primary;
  - a parked NetBox zone served by the primary;
  - a zone ignored by policy;
  - a disabled record against an inactive one (in sync).
- **Integration tests** run `nbpdns drift` on it, as a table and as JSON, and
  check each case and the exit code; also with `--zone`, and with a group
  whose primary is unreachable (exit 1, with the other group still
  reported).
- **Unit tests** cover the comparison table, policy decoding, exit codes, and
  the report's text and JSON.

##### Docs

| Section | Pages |
|---|---|
| Tutorial | "Find drift between NetBox and PowerDNS", using the lab |
| How-to | "Set a zone's drift policy" |
| Reference | Configuration and command line (generated), with the new fields and exit codes |
| Explanation | "How nbpdns finds drift": served data, the SOA serial, unmanaged and inactive zones, policies, failures and scale |

`CHANGELOG.md` lines go under Unreleased.

#### Items and phases

| Phase | Items |
|---|---|
| M3a Policy | ITEM-0042 Drift policies in the server groups' config |
| M3b Comparison | ITEM-0043 Compare zones and report drift (ADR-0027, REQ-043), with the scale benchmark |
| M3c Command | ITEM-0044 `nbpdns drift`: reading both sides, failures, exit codes, table and JSON |
| M3d Lab and docs | ITEM-0045 Drift fixture and integration tests; ITEM-0046 Drift docs and CHANGELOG |

Once the plan is approved, one commit records the design: this plan, verbatim,
under **Approved design** in M03's file; ADR-0027; the answered questions,
REQ-043 and ADR-0008's open point; the brief's dated answers; M03 marked in
progress; and the items. Each item is then committed as it's done, on
`m03-drift-report`.

#### Acceptance criteria

- [ ] ADR-0027 is accepted. Q-017 and Q-027 are answered, and REQ-043
  exists.
- [ ] Drift policies load from the config file, with strict validation, are
  shown by `config show`, and are documented in the reference.
- [ ] The comparison handles every case in ADR-0027, covered by table tests.
  The benchmark compares 1,000 zones and 100,000 records in under a second,
  and its figures are recorded.
- [ ] `nbpdns drift` reports as a table and as JSON, with `--group` and
  `--zone`, and exits 0, 3, 1 or 2 as ADR-0027 says.
- [ ] Integration tests cover every case of the drift fixture against the
  lab's NetBox 4.7 and PowerDNS 5.1, and a group that can't be read. The lab
  gains no containers.
- [ ] The docs pages above exist, the generated references are current, and
  the CHANGELOG is updated.
- [ ] `/code-review high` has run. `/security-review` runs if anything touches
  secrets.
- [ ] The manual verification is recorded. The GitLab and GitHub pipelines
  pass, and the user has merged through an MR with a merge commit.

#### Verification

- `make check` on every commit, and `make test-integration` against the lab.
- **Manual, on a fresh lab:**
  1. Follow the tutorial: put a zone in NetBox and on lab-a's primary,
     change one record on each side, and see each change in `nbpdns drift`,
     as a table and as JSON.
  2. The exit codes: 0 when in sync, 3 with drift, 1 with the primary's URL
     pointing nowhere, 2 with an unknown flag.
  3. Set `ignore` on a zone, and see it listed as ignored, not compared.
  4. Scale: generate the target, 1,000 zones of 100 records, in the lab's
     NetBox and on lab-a, with bulk API calls; run `nbpdns drift`, and record
     its time, its memory and the number of requests it made.

#### Your side

- Nothing new. The runners stay as they are, since M03 adds no containers.
