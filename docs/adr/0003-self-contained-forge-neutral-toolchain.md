---
title: "0003: Self-contained, forge-neutral toolchain"
status: superseded by ADR-0013
date: 2026-09-25
decision-makers: [jordan]
requirements: [REQ-022, REQ-026]
questions: [Q-004]
---

# 0003: Self-contained, forge-neutral toolchain

## Context and problem statement

The repository is on a self-hosted GitLab today. The user said a copy may later
live on GitHub or elsewhere, and that the project should be "as self-contained
as possible" (Q-004). The software also manages DNS infrastructure, which is
often deployed in restricted or air-gapped networks.

## Decision drivers

- Moving the repo to another forge must not lose knowledge or break the build.
- Anyone, including a fresh Claude session, can reproduce CI locally.
- Builds and the running product must not depend on third-party services
  reached at runtime (CDNs, hosted docs).

## Considered options

1. Self-contained: the logic lives in the repo, and forge CI files are thin
   wrappers around it.
2. Use forge features directly: GitLab CI logic, Issues, Pages.

## Decision outcome

Chosen option: **self-contained**.

- **Prerequisites** are Go, Docker and make only.
- **Tools are pinned in the repo.**
  - Go tools use `tool` directives in `tools/go.mod`.
  - Anything that isn't Go runs from a container image pinned by digest.
- **`make` is the only entry point.**
  - `make ci` runs the whole pipeline locally.
  - `.gitlab-ci.yml` and `.github/workflows/*.yml` only call make targets.
  - No build logic lives in forge-specific files.
- **All knowledge lives in the repo:** tracking (ADR-0002), decisions (ADR-0001)
  and documentation. The project doesn't use forge issues, wikis, Pages or
  merge-request descriptions as a record.
- **No runtime CDN.** UI assets and API-docs assets are vendored and embedded
  in the binary.
- **Releases are built by a forge-neutral tool** (GoReleaser is the candidate),
  able to publish to either forge.

### Consequences

- Good: moving forges means adding a CI wrapper file. Air-gapped installs work.
- Good: CI failures can be reproduced exactly on a laptop.
- Bad: tools that aren't Go (a docs theme needing Node, for example) need a
  pinned container image or are avoided. This favours Go-native tools.
- Bad: forge features that save time (issue boards, Pages) aren't used.

### Confirmation

- A CI job fails if a forge CI file contains commands other than make targets
  (added with ITEM-0006).
- The cold-start test in M0 checks that a fresh session can work from the repo
  alone.

## More information

- ITEM-0004: tool pinning.
- ITEM-0006: Makefile and CI wrappers.
