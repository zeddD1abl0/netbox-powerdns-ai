---
title: Run nbpdns on PostgreSQL
weight: 36
---

# Run nbpdns on PostgreSQL

`nbpdns serve` keeps its state in SQLite by default, in a file beside it.
This guide keeps it in a PostgreSQL database instead: it makes a role and a
database for nbpdns, writes `database.url` with TLS, gives nbpdns the
password without putting it on a command line, and starts `serve` on it.

The state is the same on either database: the settings, the server groups,
the secrets that nbpdns stores, the audit trail, and the drift history. An
install that moves to PostgreSQL starts with an empty database: nbpdns
doesn't copy a SQLite database's state across.

> [!NOTE]
> One `serve` uses a database at a time, as on SQLite: a second one refuses
> to start, and names the first. Several replicas of `serve` on one
> PostgreSQL database come in M09.

## Before you start

You need:

- PostgreSQL 18, which [Supported versions](../reference/supported-versions.md#postgresql)
  lists, reachable from the host that runs nbpdns;
- `psql`, as a role that can create roles and databases, such as
  `postgres`;
- nbpdns, installed as [Install nbpdns](install-nbpdns.md) or
  [Run nbpdns in a container](run-nbpdns-in-a-container.md) describes. With
  PostgreSQL, `serve` needs no directory or volume for its state.

## Make a role and a database

1. Make a role for nbpdns that can log in, and give it a password. psql's
   `\password` asks for it, so it stays out of your shell's history and the
   server's log:

   ```shell
   psql -h db.example.com -U postgres -c 'CREATE ROLE nbpdns LOGIN'
   psql -h db.example.com -U postgres -c '\password nbpdns'
   ```

2. Make a database that the role owns. nbpdns creates its tables in its
   `public` schema, which the database's owner can write to:

   ```shell
   psql -h db.example.com -U postgres -c 'CREATE DATABASE nbpdns OWNER nbpdns'
   ```

3. Let only nbpdns's role connect to it:

   ```shell
   psql -h db.example.com -U postgres -c 'REVOKE CONNECT ON DATABASE nbpdns FROM PUBLIC'
   ```

Give nbpdns a database of its own, which it shares with nothing else, not
even NetBox.

## Write `database.url`

`database.url` takes PostgreSQL's own URL, as `psql` and libpq read it, with
the same parameters and defaults:

```text
postgres://nbpdns@db.example.com:5432/nbpdns?sslmode=verify-full&sslrootcert=/etc/nbpdns/pg-ca.pem
```

| Parameter | What it does |
|---|---|
| `sslmode` | Whether to use TLS, and what to check. `verify-full` encrypts, and checks that the server's certificate is valid and names the host in the URL. The default, `prefer`, uses TLS only if the server offers it, and checks nothing. |
| `sslrootcert` | A PEM file of the CA certificates that the server's certificate must come from, or `system` for the system's CAs. Without it, nbpdns reads `~/.postgresql/root.crt`, as libpq does, if it exists, and trusts the system's CAs if it doesn't. |
| `sslcert`, `sslkey` | A client certificate and its key, as PEM files, for a server that asks for one. Without them, nbpdns reads `~/.postgresql/postgresql.crt` and `postgresql.key`, if both exist. |
| `sslpassword` | The key's passphrase. nbpdns reads it for an RSA key encrypted in OpenSSL's traditional format only, such as `openssl rsa -aes256 -traditional` writes. |
| `passfile` | A password file, in libpq's `.pgpass` form. Without it, nbpdns reads `~/.pgpass`, if it exists. |
| `host` | A Unix socket's directory, such as `/var/run/postgresql`, in place of the URL's host. |
| `connect_timeout` | How long to wait for the server, in seconds. By default, nbpdns waits as long as the operating system does. |

The scheme can be `postgres://` or `postgresql://`. Percent-encode a
character in the password that a URL reserves, such as `@` as `%40`, or `#`
as `%23`.

libpq's environment variables, such as `PGHOST`, `PGSSLMODE` and
`PGPASSWORD`, fill in what the URL leaves out. The URL's own values win.

nbpdns shows and logs the URL with each password in it masked as `xxxxx`:
in `nbpdns config show`, in `nbpdns db status`, in the logs, and in
`/status`.

> [!WARNING]
> With `sslmode` at `prefer`, `allow`, or `disable`, nbpdns sends the
> password and the data unencrypted wherever the server doesn't offer TLS,
> and logs a warning each time it connects that way. Below `verify-full`,
> nothing checks that the server is the one that the URL names. Use
> `sslmode=verify-full`.

## Give nbpdns the password

Give it in one of these ways. Never put it on the command line, with
`--database-url`, where other users of the host can see it in the process
list.

- **The whole URL, in a file:** write the URL, password and all, in a file
  that only nbpdns's user can read, and name the file in
  `NBPDNS_DATABASE_URL_FILE`, `--database-url-file`, or `database.url_file`
  in the config file:

  ```shell
  (umask 077 && printf '%s\n' 'postgres://nbpdns:P%40ssw0rd@db.example.com/nbpdns?sslmode=verify-full&sslrootcert=/etc/nbpdns/pg-ca.pem' \
    > /etc/nbpdns/secrets/database-url)
  ```

  ```yaml
  database:
    url_file: /etc/nbpdns/secrets/database-url
  ```

  In a container, mount the file as a secret, as
  [Run nbpdns in a container](run-nbpdns-in-a-container.md) does the others.

- **A password file:** keep the URL without its password, in the config file,
  and name a password file with `passfile`. Each line is
  `host:port:database:user:password`, and `*` matches anything:

  ```shell
  (umask 077 && printf '%s\n' 'db.example.com:5432:nbpdns:nbpdns:P@ssw0rd' > /etc/nbpdns/secrets/pgpass)
  ```

  ```yaml
  database:
    url: postgres://nbpdns@db.example.com/nbpdns?sslmode=verify-full&sslrootcert=/etc/nbpdns/pg-ca.pem&passfile=/etc/nbpdns/secrets/pgpass
  ```

  The password isn't percent-encoded in a password file. Escape a `:` or a
  `\` in it with `\`.

- **A client certificate:** with a server whose `pg_hba.conf` authenticates
  nbpdns by its certificate, the URL needs no password at all. Add
  `sslcert` and `sslkey`, and keep the key readable by nbpdns's user only.

## Create the tables

1. Apply the migrations, which create nbpdns's tables in the database:

   ```shell
   nbpdns db migrate
   ```

   ```text
   Applied migration 1, database.
   Applied migration 2, audit.
   Applied migration 3, drift_history.
   Applied migration 4, settings.
   Applied migration 5, server_groups.
   The database, postgres://nbpdns:xxxxx@db.example.com/nbpdns?sslmode=verify-full&sslrootcert=/etc/nbpdns/pg-ca.pem, is up to date, at version 5.
   ```

   `serve` applies them too, when it starts, so this step is optional. It
   checks the URL before `serve` relies on it.

2. Check the connection:

   ```shell
   nbpdns db status
   ```

   ```text
   Database: PostgreSQL 18.1, postgres://nbpdns:xxxxx@db.example.com/nbpdns?sslmode=verify-full&sslrootcert=/etc/nbpdns/pg-ca.pem, over TLS
   ID:       I5S4DUBY7JZSMIXNGZ4W2RX5L7, created 2026-10-18T14:15:18Z by nbpdns v0.9.0
   Schema:   version 5, up to date
   ```

   `over TLS` says that the connection is encrypted. `unencrypted` says
   that it isn't, as `sslmode=prefer` allows with a server that doesn't
   offer TLS.

If nbpdns can't connect, it says why, such as `server refused TLS
connection` for `sslmode=require` with a server that has no TLS, or
`password authentication failed for user "nbpdns"`.

## Start serve

Start `serve` as you would on SQLite. It logs `opened the database`, with
the URL, masked, and whether the connection uses TLS. Its `/status` shows
the same, in its `database` section:

```json
"database": {
  "backend": "postgresql",
  "url": "postgres://nbpdns:xxxxx@db.example.com/nbpdns?sslmode=verify-full&sslrootcert=/etc/nbpdns/pg-ca.pem",
  "path": "",
  "tls": true,
  …
}
```

`serve` holds a lock in the database, a PostgreSQL session advisory lock,
while it runs. A second `serve` on the same database refuses to start, as
`nbpdns db migrate` and `nbpdns secrets rotate-master-key` do:

```text
another nbpdns holds the database's lock: host nbpdns-1, PID 4242, since 2026-10-18T14:15:27Z; stop it first, or give this one another database.url
```

PostgreSQL releases the lock when `serve`'s session ends, however it ends.

## Connect through a pooler

nbpdns keeps a few connections open, and needs its sessions to itself: its
lock lasts as long as the session that took it. Connect directly to
PostgreSQL, or through a pooler in session mode, such as PgBouncer with
`pool_mode = session`. A pooler in transaction mode can't hold the lock.

## Next steps

- [Back up nbpdns's database](back-up-nbpdns-database.md#back-up-a-postgresql-database)
  copies it with `pg_dump`.
- [Protect stored secrets with a master key](protect-stored-secrets-with-a-master-key.md):
  the secrets that the database stores are sealed, on PostgreSQL as on
  SQLite.
- The [configuration reference](../reference/configuration.md#databaseurl)
  describes `database.url`.
