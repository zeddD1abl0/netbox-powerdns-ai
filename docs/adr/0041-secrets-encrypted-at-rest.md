---
title: '0041: Secrets encrypted at rest'
status: proposed # proposed | accepted | rejected | deprecated | superseded by ADR-NNNN
date: 2026-10-09
decision-makers: [jordan]
requirements: [REQ-051, REQ-053]
questions: [Q-023]
supersedes: # ADR-NNNN this replaces, if any
---

# 0041: Secrets encrypted at rest

## Context and problem statement

From M08, the database can hold secrets: a runtime setting such as
`netbox.token`, and the API key of a group added with the CLI (ADR-0040).
Q-023 asked how secrets are stored at rest; its proposed default is envelope
encryption with a master key from the environment or a file, and Vault or
OpenBao later. The user chose the ISM, among others, on 2026-10-09, so the
algorithms must be ASD-approved.

## Decision drivers

- A copy of the database must not give away its secrets.
- ASD-approved algorithms (REQ-053).
- The master key rotates without re-entering every secret.
- No secret needs a key until one is stored.

## Considered options

1. Envelope encryption, with a master key from a file or the environment.
2. One key for every secret.
3. Vault or OpenBao now.

## Decision outcome

Chosen: **envelope encryption** (Q-023's default).

- **Sealing:** each stored secret has its own random data key. The value is
  sealed with AES-256-GCM, and the data key is wrapped with the master key,
  also with AES-256-GCM. Both are ASD-approved. The master key's ID is
  stored beside each wrapped key.
- **The master key:** `database.master_key`, a bootstrap secret with a
  `_FILE` form: 32 random bytes, in base64. It's needed only once a secret is
  stored. Without it, storing one fails, and `serve` refuses to start if the
  database holds secrets it can't open.
- **What's encrypted:** secret runtime settings, and the API keys of
  CLI-managed groups. A file-managed group's key stays in the file.
- **Rotation:** `nbpdns secrets rotate-master-key --new-key-file` re-wraps
  every data key in one transaction, and is audited (ADR-0039).
- Vault and OpenBao are left for later, as Q-023 says.

### Consequences

- Good: a stolen database file without its master key gives away no secret.
- Good: rotating the master key re-wraps small data keys, not every value.
- Bad: the master key must be kept apart from the database, and backed up;
  losing it loses every stored secret.

### Confirmation

- Tests seal and open values, refuse a wrong key, detect a changed
  sealed value, and rotate the master key with every secret still readable.
- A test checks that no secret is written to the database unsealed.

## Pros and cons of the options

### One key for every secret

- Good: simpler.
- Bad: rotating it re-encrypts every value, and one nonce reused breaks them
  all.

### Vault or OpenBao now

- Good: keys never sit beside nbpdns.
- Bad: an outside service for the single binary, which ADR-0009 keeps
  self-contained.

## More information

- ADR-0038 (the store), ADR-0039 (audit), ADR-0040 (what's stored).
- Recorded in M08's design, 2026-10-09, on `plan-m08-m12` (ADR-0037).
  Implemented by ITEM-0092.
