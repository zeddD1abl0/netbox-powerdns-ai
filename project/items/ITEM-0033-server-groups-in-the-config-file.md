---
id: ITEM-0033
title: Server groups in the config file
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M02
requirements: [REQ-006, REQ-029, REQ-030, REQ-042]
depends_on: []
created: 2026-10-06
closed: 2026-10-06
---

# ITEM-0033: Server groups in the config file

## Goal

Server groups are declared in the config file (ADR-0024): each a name, the
NetBox views it serves, and its primary's URL, API key or key file, server
ID, CA file and client certificate. The key registry handles only scalar
keys, so it gains a list key with a field schema, so that groups are decoded
strictly, validated, shown by `config show` and documented in the generated
reference, like every other key.

## Acceptance criteria

- [x] `powerdns.timeout` and `powerdns.concurrency` are registry keys, with env variables and flags.
- [x] `powerdns.groups` loads from the config file only. An unknown field, a missing name, URL or view, a bad name or URL, both or neither of `api_key` and `api_key_file`, `cert_file` without `key_file`, and a repeated name are each errors, all reported together.
- [x] `config show` lists each group field with the file as its source, and API keys as `[redacted]`, in table and JSON.
- [x] The configuration reference documents the groups, says they come from the file only, and warns about `http://` and the key's power.
- [x] Table-driven tests cover all of it.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Created from M02's approved design.
- 2026-10-06: Done.
  - **`internal/config/groups.go`:** `GroupFields` is the one declaration of
    a group's fields (name, views, and the primary's url, api_key,
    api_key_file, server_id, ca_file, cert_file and key_file), each with its
    type, default, summary, parser and display. It drives decoding,
    `config show` and the reference. `Config.PowerDNS` holds the groups,
    with `Group(name)` to find one.
  - **Decoding:** Viper reads `powerdns.groups` as one leaf key, with the
    keys inside its entries made lowercase, so the registry's unknown-key
    check accepts the list and `decodeGroup` checks its fields. Every
    problem is reported, labeled with the entry's index and given name,
    such as `powerdns.groups[1] (site-b)`, and a field that fails to parse
    isn't also reported as missing. A key from `primary.api_key_file` is
    read like the NetBox token's file, and `Primary.APIKeyFile` keeps its
    path for `config show`.
  - **`config show`** lists `powerdns.groups` as a default row when no group
    is declared, or one row per field per group, with the file or the
    default as its source and the key redacted. Settings are now sorted by
    key, so the group rows fall between `powerdns.concurrency` and
    `powerdns.timeout`.
  - `NBPDNS_POWERDNS_GROUPS` is an unknown variable, like any other, since
    groups have no environment form.
  - Docs: the generated configuration reference has a `powerdns.groups`
    section with its fields, a warning about the key and `http://`, and an
    example; the command-line reference has the two new flags; and
    "Configuration sources and precedence" explains why groups come from the
    file only.
