---
title: "0005: Project identity — name, module path, env prefix, license"
status: accepted
date: 2026-09-25
decision-makers: [jordan]
requirements: [REQ-001, REQ-026, REQ-035]
questions: [Q-005, Q-055]
---

# 0005: Project identity — name, module path, env prefix, license

## Context and problem statement

Before any Go code can exist, the project needs:
- a Go module path, which is baked into every import and painful to change;
- a binary name and env var prefix, which operators type and script against;
- a license, which also limits which dependencies are allowed.

The repository is named `netbox-powerdns-ai`. It's on GitLab today, but must
stay forge-neutral (ADR-0003).

## Decision drivers

- A stable import path, even if the repo moves forges.
- A name operators can type, and one that doesn't imply AI features the product
  doesn't have.
- A license suited to an infrastructure tool, with Terraform and Ansible
  integrations planned.

## Considered options

- **Name:** `netbox-powerdns-ai` (the repo name), or `nbpdns`.
- **Module path:**
  - a vanity path, for example `go.itctsv.com/...`;
  - the GitLab path;
  - a GitHub path.
- **License:** proprietary, Apache-2.0, MIT or AGPL-3.0.

## Decision outcome

The user chose these on 2026-09-25 (Q-005):

| Item | Value |
|---|---|
| Product name | `nbpdns` |
| Binary | `nbpdns`, built from `cmd/nbpdns` |
| Env var prefix | `NBPDNS_`, for example `NBPDNS_DATABASE_URL` or `NBPDNS_LOG_LEVEL` |
| Go module path | `github.com/zeddD1abl0/netbox-powerdns-ai`. The repo keeps its current name; the owner comes from the `github` remote (Q-055). |
| License | Apache-2.0 |

The owner's mixed casing (`zeddD1abl0`) is kept exactly as the GitHub remote
shows it. Go module paths are case-sensitive. Every import must use this
spelling, and the module cache stores it escaped as `zedd!d1abl0`.

### Consequences

- Good: Apache-2.0 is permissive and includes a patent grant, and it's the norm
  for Terraform providers and Go infrastructure tools. The dependency
  allowlist (MIT, BSD, Apache-2.0, MPL-2.0) is compatible with it.
- Good: the short binary name and prefix are easy to type. The product name
  doesn't suggest AI features.
- Bad: the module path points at GitHub while the repo lives on GitLab.
  - Building inside the repo works regardless, because the path is only a
    name.
  - **External** consumers (the Terraform provider in M7, or anyone importing
    a client package) resolve it from GitHub. The repository exists there as
    the `github` remote, so the code must be pushed to it before anything
    imports the module.
- Bad: the repo name (`netbox-powerdns-ai`) and the product name (`nbpdns`)
  differ. The README says so up front.

### Confirmation

- `go.mod` declares the module path (ITEM-0004).
- `LICENSE` holds the unmodified Apache-2.0 text.
- The config registry enforces the `NBPDNS_` prefix, and generated docs list
  every key with it.

## More information

- Q-055: the GitHub owner, answered 2026-09-25.
