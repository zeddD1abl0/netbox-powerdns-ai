---
id: M11
title: Roles and permissions
status: planned # planned | in-progress | done
started:
closed:
---

# M11: Roles and permissions

A roadmap entry, written ahead on `plan-m08-m12` (ADR-0037), and re-planned
from "SSO and RBAC" by ADR-0043. Its design, items and ADRs come in a
plan-mode session before it starts.

## Goal

Control who can do what: every action through the API is checked against
roles, for tokens and service accounts, before anyone signs in (REQ-057).

## Scope

- **Permissions:** one per action, declared once in a registry, with the
  permission reference generated from it (CLAUDE.md, principle 2).
- **Roles (Q-030):** Viewer, Operator and Admin built in, and custom roles
  made of permissions.
- **Grants (Q-015):** a role granted globally, or for chosen server groups,
  to a service account or a token, replacing M10's coarse scopes.
- **The API's writes** for settings and groups, as `managed_by=api`, each
  checked against a permission, with ETag and If-Match, and an
  `Idempotency-Key` (ADR-0012), if M10 hasn't added them.
- **Managing roles and grants** with the CLI and the API, every change
  audited.

## Non-goals

- **No people signing in:** M12, which maps identity-provider groups to
  these roles.
- **No tenant scoping:** a later ADR (Q-015).
- **No UI:** M13.

## Dependencies

- M10: tokens and service accounts.
- M08 and M09: the database and the audit trail.

## Decided, 2026-10-09

- Built-in and custom roles, each permission checked per action, granted
  globally or by server group (Q-030, Q-015, REQ-057).
- Roles come before sign-in, so that every session starts with them
  (ADR-0043).

## Open for its design

- The permission list, action by action, and what each built-in role holds,
  before the write path, M14, adds its actions.
- Whether a changed grant reaches a live token at once.
- Where the API's writes land, if M10 left them.
