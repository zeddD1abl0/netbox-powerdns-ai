---
id: ITEM-0092
title: Secrets encrypted at rest, and master key rotation
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M08
requirements: [REQ-051, REQ-053]
depends_on: [ITEM-0090]
created: 2026-10-09
closed: 2026-10-14
---

# ITEM-0092: Secrets encrypted at rest, and master key rotation

## Goal

Envelope encryption (ADR-0041): AES-256-GCM data keys wrapped by the
master key, `database.master_key` with its `_FILE` form; secret runtime
settings and CLI groups' API keys sealed; `serve` refusing secrets it can't
open; and `nbpdns secrets rotate-master-key`, audited.

## Acceptance criteria

- [x] No secret is written to the database unsealed, as a test checks.
- [x] A wrong key and a changed sealed value are refused.
- [x] Rotating the master key leaves every secret readable.
- [x] A secret sealed with another master key than the database's is refused, so a rotation can't race a `settings set`.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-10-09: Created from M08's approved design, on `plan-m08-m12`.
- 2026-10-10: Done before ITEM-0091, which now depends on this item
  rather than the other way round: a CLI-managed group's API key must be
  sealed when it's first stored, so the sealing has to exist first. This
  item seals the secret runtime settings; ITEM-0091 seals the groups' API
  keys with it, and its rotation covers them.
- 2026-10-10: `internal/secrets`: `Seal` makes a random data key per
  value, seals the value with AES-256-GCM, and wraps the data key with the
  master key, AES-256-GCM too, each with a random nonce and with additional
  data naming what it is and its place, such as `setting:netbox.token`, so
  that a value moved to another place doesn't open. The stored form is
  JSON with the master key's ID (16 hex digits of a SHA-384 of the key),
  the wrapped key and the ciphertext. `Open` returns a `WrongKeyError`
  naming both keys, or `ErrTampered`; `Rewrap` re-wraps the data key only.
- 2026-10-10: `database.master_key`, a bootstrap secret with its `_FILE`
  forms, checked as 32 bytes of base64. Migration 4's `settings` gained
  `sealed`: it hadn't shipped in a release, so it was changed rather than
  followed by a fifth. `ChangeSetting` refuses a secret that isn't in
  sealed form, whatever its caller does. `RotateMasterKey` re-wraps every
  sealed value and records `master_key.rotated`, in one transaction, so a
  value that doesn't open changes nothing.
- 2026-10-10: CLI: `nbpdns settings set KEY --from-file FILE|-` for a
  secret, which is refused on the command line, sealed with the master
  key, and refused without one; settings are opened with the master key
  when they're applied, so `serve` refuses to start with a secret it can't
  open. `nbpdns secrets rotate-master-key --new-key-file`, which takes the
  serve lock, since serve opens the secrets with the old key. The webhook
  secret is live: the API reads it from serve's live configuration at each
  request, and `/status` says whether webhooks are on. Tests seal, refuse a
  wrong key, a changed value or data key, and a moved value, rotate, and
  scan the database's files and the audit trail for the plaintext. The
  how-to "Protect stored secrets with a master key".
- 2026-10-14: The session's review found that `settings set`, which
  doesn't take serve's lock, could store a secret sealed with the old key
  after a rotation, or while one ran, leaving a database that serve can't
  open with either key. The user chose to fix it in this item.
  `ChangeSetting` now refuses, in its transaction, a secret sealed with
  another master key than every sealed value stored, the one it replaces
  included. Transactions are immediate, so a rotation can't come between
  the check and the write. Once no secret is stored, any key seals the
  first, so the lost-key recovery in the how-to still works. The store's
  and the CLI's tests failed without the check. Also asked: whether
  `settings list` and `get` should work without the master key; the user
  said no, so they still open every secret. `make check` is green. Done.
