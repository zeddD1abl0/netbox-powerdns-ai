---
id: M10
title: Authentication
status: planned # planned | in-progress | done
started:
closed:
---

# M10: Authentication

A roadmap entry, written ahead on `plan-m08-m12` (ADR-0037). Its design,
items and ADRs come in a plan-mode session before it starts.

## Goal

Lock down nbpdns's HTTP endpoints with tokens before anything can change
state from outside: the API, `/status` and `/metrics` need a token, and only
the probes stay open.

## Scope

- **API tokens** for scripts, IaC and CI: hashed, scoped and expiring (Q-031),
  each owned by a **service account**. They're created, listed and revoked
  with the local CLI, as M08's settings are, and every use of a token that
  changes something is audited.
- **The break-glass token:** a bootstrap secret, from the environment or a
  file only, for when nothing else works. Every use is audited (Q-029).
- **What needs a token:** every `/api` path, `/status` and `/metrics`.
  Prometheus scrapes with a token of its own. Only `/livez` and `/readyz` stay
  open (REQ-055).
- **Rate limits** per token and per client address; failed authentications
  count against the address (Q-040).
- **The audit trail in the API,** behind authentication (ADR-0039).
- **Host checks:** requests whose `Host` isn't `server.public_url`'s are
  refused (ITEM-0074), against DNS rebinding.

## Non-goals

- **No local users, MFA or sessions:** they need a sign-in page, so they come
  with the UI, in M12. There, MFA is WebAuthn only (REQ-056).
- **No SSO, no roles:** M11. Until then, a token's scope is its only limit.
- **No OIDC workload identity** for CI: later (Q-031).
- **No UI:** M12.

## Dependencies

- M08: the database, for tokens and service accounts, and the audit trail.
- M09: replicas share the tokens through PostgreSQL.

## Decided, 2026-10-09

- Local users, MFA and sessions move to M12, with the sign-in page they need.
- MFA is WebAuthn only: passkeys and security keys, no TOTP, as the
  Essential Eight asks (REQ-056, Q-029).
- Only `/livez` and `/readyz` are open without a token (REQ-055).
- Rate limits per token and per address (Q-040).

## Open for its design

- Q-031: the tokens' scopes before M11's roles, such as read and admin, and
  their longest lifetime.
- Whether M10 adds API writes for settings and groups, as `managed_by=api`,
  or they wait for M11's roles.
- TLS: on nbpdns's own listener, or only at a proxy in front of it.
- How a token is sent and checked in the OpenAPI document, as its security
  scheme.
