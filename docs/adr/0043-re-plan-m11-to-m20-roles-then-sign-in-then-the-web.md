---
title: '0043: Re-plan M11 to M20: roles, then sign-in, then the web UI'
status: accepted # proposed | accepted | rejected | deprecated | superseded by ADR-NNNN
date: 2026-10-09
decision-makers: [jordan]
requirements: []
questions: [Q-015, Q-016, Q-028, Q-030, Q-032, Q-050, Q-051]
supersedes: # ADR-NNNN this replaces, if any
---

# 0043: Re-plan M11 to M20: roles, then sign-in, then the web UI

## Context and problem statement

ADR-0019 set the milestones, and ADR-0028 split packaging into its own, so
that M11 was SSO and RBAC, M12 the web UI, and M13 to M19 the write path to
v1.0. Writing their roadmap entries on `plan-m08-m12` (ADR-0037) on
2026-10-09 found that the order didn't hold:

- **Sign-in needs a page.** Local users with WebAuthn, and SSO through OIDC,
  SAML or proxy headers, are browser sign-ins, with sessions. M10's roadmap
  had already moved local users, MFA and sessions to the UI, in M12, since
  M10 has no page to sign in on; SSO in M11 had the same problem.
- **M12 grew too large.** With sign-in moved into it, M12 held the UI, local
  users, sessions, three sign-in protocols, group mapping and SCIM.

## Decision drivers

- Each milestone is one useful, logical step.
- Roles must guard the API's writes as soon as they exist.
- A human signs in where the UI is, with every sign-in in one place.

## Considered options

1. Roles in M11, sign-in in M12, the UI in M13, renumbering the rest.
2. Roles and sign-in in M11, with a bare page, and the UI in M12.
3. As planned, with SSO in M11 waiting for M12's page.
4. Roles in M11, and sign-in and the UI together in M12.

## Decision outcome

Chosen: **roles in M11, sign-in in M12, the UI in M13**, as the user chose,
with every milestone after them moved down by one.

| Before | Now |
|---|---|
| M11 SSO and RBAC | M11 Roles and permissions |
| M12 Web UI | M12 Sign-in |
| — | M13 Web UI |
| M13 Write path: plan and apply | M14 Write path: plan and apply |
| M14 Change safety | M15 Change safety |
| M15 Brownfield import | M16 Brownfield import |
| M16 SIEM export | M17 SIEM export |
| M17 Production hardening | M18 Production hardening |
| M18 Terraform/OpenTofu provider | M19 Terraform/OpenTofu provider |
| M19 Ansible collection and v1.0 | M20 Ansible collection and v1 |

- **M11, Roles and permissions:** built-in and custom roles, each permission
  checked per action, granted globally or for chosen server groups (Q-030,
  Q-015), for tokens and service accounts, and the API writes they guard.
- **M12, Sign-in:** local users with WebAuthn only; sessions; OIDC, SAML and
  trusted proxy headers, each turned on separately, with Authentik as the
  reference provider (Q-028); identity-provider groups mapped to roles; and
  SCIM provisioning (Q-032); with a bare sign-in page.
- **M13, Web UI:** Q-016's scope, in templ and htmx (Q-050), to WCAG 2.2 AA,
  in English (Q-051).
- M20's title says v1, not v1.0, since ADR-0037 numbers stable releases
  `v1.NN.0`.
- The planned milestones' files are renamed, or renumbered, to match. Their
  GitLab milestones are retitled. Accepted ADRs, done milestones, items'
  notes and the brief keep the numbers they were written with; this table
  maps them. Everything else names the new numbers.

### Consequences

- Good: each of M11, M12 and M13 is one step, and v1 lands at M20, where the
  user expects to start using nbpdns.
- Good: roles exist before anything signs in, so every session starts with
  them.
- Bad: a milestone more before v1.
- Bad: older records name the old numbers, so a reader of them needs this
  table.

### Confirmation

- `make project-lint` checks that each milestone file's ID matches its name,
  and each item's milestone exists.
- The board, the GitLab milestones and the requirements' "Needed by" all
  show the new numbers.

## Pros and cons of the options

### Roles and sign-in in M11, the UI in M12

- Good: no renumbering.
- Bad: M11 as large as M12 would have been, and a sign-in page without a UI.

### As planned

- Good: no change.
- Bad: SSO with nowhere to sign in, for a whole milestone.

### Sign-in and the UI together in M12

- Good: no renumbering.
- Bad: the largest milestone of all.

## More information

- ADR-0019 (the milestones), ADR-0028 (packaging's split), ADR-0037
  (planning ahead, and versions).
- Recorded on `plan-m08-m12`, 2026-10-09, with M11 to M13's roadmap entries.
