---
id: ITEM-0033
title: Server groups in the config file
type: feature # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M02
requirements: [REQ-006, REQ-029, REQ-030, REQ-042]
depends_on: []
created: 2026-10-06
closed:
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

- [ ] `powerdns.timeout` and `powerdns.concurrency` are registry keys, with env variables and flags.
- [ ] `powerdns.groups` loads from the config file only. An unknown field, a missing name, URL or view, a bad name or URL, both or neither of `api_key` and `api_key_file`, `cert_file` without `key_file`, and a repeated name are each errors, all reported together.
- [ ] `config show` lists each group field with the file as its source, and API keys as `[redacted]`, in table and JSON.
- [ ] The configuration reference documents the groups, says they come from the file only, and warns about `http://` and the key's power.
- [ ] Table-driven tests cover all of it.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-06: Created from M02's approved design.
