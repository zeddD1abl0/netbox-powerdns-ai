---
id: ITEM-0134
title: Mask a database URL's password as pgx reads it
type: bug # feature | bug | debt | task
status: open # open | in-progress | blocked | done | wontfix
milestone: M10
requirements: [REQ-054]
depends_on: []
created: 2026-11-26
closed:
---

# ITEM-0134: Mask a database URL's password as pgx reads it

## Goal

nbpdns shows and logs `database.url` with its password masked (ADR-0046),
and finds the password with Go's `net/url`. pgx reads the URL as libpq
does, where only a `/` ends the search for the `@` after the user
information. Take a password that starts with `?`, or is only digits
before a `?`, such as `postgres://nbpdns:?Secr3t@db.example.com/nbpdns`.
`net/url` reads `nbpdns:` as the host and the rest as the query, so nbpdns
accepts the URL, finds no password, and shows it whole, while pgx connects
with the password `?Secr3t`. The password then appears in:

- `/status` and `/api/status`, which anyone who can reach `server.listen`
  can read until M10;
- the `opened the database` log line, and the warning of an unencrypted
  connection;
- `db status` and `config show`.

With `#` in that place, the error that refuses the URL prints it whole.

## Acceptance criteria

- [ ] For every URL that nbpdns accepts, it masks the password that pgx
  would use. Either it masks from pgx's own reading of the URL, or it
  refuses a URL whose user information `net/url` and pgx read differently.
- [ ] No error that refuses a `database.url` holds its password.
- [ ] The test that the password is in no log line, error or `/status`
  field covers `?Secr3t`, `2024?Secr3t`, `#Secr3t`, and a user name with
  `?`.
- [ ] The CHANGELOG has a line under Security.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-11-26: Found by M09's `/security-review`, on
  `main...m09-postgresql-and-ha`, at confidence 6 of 10. That is below the
  review's bar of 8, only because it needs a password that should have
  been percent-encoded. The mechanism is certain:
  `checkDatabaseURL` and `maskURL` in `internal/config/key.go` use
  `url.Parse`, and pgx v5.11.0's `parseURLSettings`
  (`pgconn/parse_url.go`) cuts the user information at the first `@`
  before a `/`. Left for after M09, as the user chose. v0.9.0 ships with
  it; it's the first item to take in M10.
