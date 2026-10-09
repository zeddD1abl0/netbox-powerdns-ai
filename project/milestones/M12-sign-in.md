---
id: M12
title: Sign-in
status: planned # planned | in-progress | done
started:
closed:
---

# M12: Sign-in

A roadmap entry, written ahead on `plan-m08-m12` (ADR-0037), and re-planned
from "Web UI" by ADR-0043. Its design, items and ADRs come in a plan-mode
session before it starts.

## Goal

People sign in to nbpdns: with a local account and WebAuthn, or through the
organization's identity provider, with roles from their groups (REQ-058).

## Scope

- **A sign-in page:** server-rendered, bare, the one page before M13's UI.
- **Sessions:** secure cookies, with idle and absolute timeouts and CSRF
  protection, kept in the database so that every replica knows them.
- **Local users with WebAuthn only** (REQ-056): passkeys and security keys,
  no TOTP. The break-glass token still works when nothing else does.
- **SSO (Q-028):** OIDC, SAML and trusted proxy headers, each turned on
  separately, with Authentik as the reference provider. Proxy headers are
  trusted only from configured addresses.
- **Groups to roles:** identity-provider groups mapped to M11's roles.
- **SCIM provisioning (Q-032):** the identity provider pushes users and
  groups, and someone it removes loses their sessions at once.
- **Every sign-in, sign-out and failure audited,** and failures rate-limited
  (M10).

## Non-goals

- **No UI beyond the sign-in page:** M13.
- **No LDAP, no TOTP.**
- **No tenant scoping.**

## Dependencies

- M11: the roles that groups map to.
- M10: tokens, the break-glass token and rate limits.
- M08 and M09: the database, for users and sessions, and the audit trail.

## Decided, 2026-10-09

- Sign-in, local and SSO, comes in one milestone, with its page
  (ADR-0043).
- OIDC, SAML and proxy headers (Q-028), and SCIM (Q-032).
- WebAuthn only for local users (REQ-056).

## Open for its design

- The SAML library, by its security record, and the WebAuthn library.
- Session timeouts by default, under the ISM and the Essential Eight.
- Recovering a local user who loses their authenticators.
- SCIM's scope: users and groups, and what a removed user's tokens do.
