---
id: M02
title: Identity
status: planned # planned | in-progress | done
started:
closed:
---

# M02: Identity

## Goal

Who can do what: authentication, authorization and machine access, all audited.

## Scope (provisional)

- Local users with MFA, and an env-only break-glass token
- SSO: OIDC, SAML and trusted proxy headers (Authentik as the reference IdP)
- RBAC: built-in and custom roles, IdP group mapping, permissions checked per action, a generated permission reference
- API tokens and service accounts for IaC and CI, and sessions

## Design, non-goals and acceptance criteria

To be written in this milestone's plan-mode session, before implementation
starts. The milestone's branch is `m02-identity` (ADR-0010).
