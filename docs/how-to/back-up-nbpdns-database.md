---
title: Back up nbpdns's database
weight: 48
---

# Back up nbpdns's database

This guide copies `nbpdns serve`'s database while it runs, checks the copy,
and restores one. For SQLite, it covers a database on the host, as
[Install nbpdns](install-nbpdns.md#make-the-databases-directory) makes it,
or bind-mounted into a container, as
[Run nbpdns in a container](run-nbpdns-in-a-container.md#run-it-with-docker)
does. [How nbpdns stores its state](../explanation/how-nbpdns-stores-its-state.md#copies-of-the-database)
explains why copying the file of a running database isn't safe. For
PostgreSQL, as [Run nbpdns on PostgreSQL](run-nbpdns-on-postgresql.md)
sets it up, it uses `pg_dump`: see
[Back up a PostgreSQL database](#back-up-a-postgresql-database).

> [!IMPORTANT]
> Back up before you upgrade nbpdns. Migrations only move forward, so a copy
> made before the upgrade is the only way back to the older release.

## Back up a SQLite database

### Before you start

You need:

- root on the host, through `sudo`;
- `sqlite3` on the host, such as from Debian's `sqlite3` package. The image
  doesn't have it;
- the database's path, from `database.url`. This guide uses the default,
  `/var/lib/nbpdns/nbpdns.db`.

### Copy the database

1. Make a directory for the copies that only root can read. A copy holds the
   audit trail, the drift history, and the stored secrets, sealed:

   ```shell
   sudo install -d -m 0700 /var/backups/nbpdns
   ```

2. Copy the database with SQLite's backup command:

   ```shell
   sudo sqlite3 /var/lib/nbpdns/nbpdns.db ".backup /var/backups/nbpdns/nbpdns-2026-10-15.db"
   ```

   The copy holds every change committed so far, whether `serve` is running
   or not, and `serve` carries on. Running it as root is safe, since SQLite
   gives any file it makes beside the database the database's owner, so
   `serve` can still open it.

Without `sqlite3`, stop `serve` first, such as with `docker stop nbpdns` or
`sudo systemctl stop nbpdns`. Once it has stopped, the database is the one
file, `nbpdns.db`, with no `nbpdns.db-wal` beside it, and
`sudo cp -p /var/lib/nbpdns/nbpdns.db /var/backups/nbpdns/nbpdns-2026-10-15.db`
copies it. Then start `serve` again.

### Check the copy

Verify the copy's audit trail, which reads every event in it. With nbpdns on
the host:

```shell
sudo nbpdns audit verify --database-url sqlite:///var/backups/nbpdns/nbpdns-2026-10-15.db
```

With Docker, run the image on the copy's directory, as root, who owns the
copy:

```shell
docker run --rm --user 0 --volume /var/backups/nbpdns:/backup \
  registry.example.com/group/netbox-powerdns-ai:0.8.0 \
  audit verify --database-url sqlite:///backup/nbpdns-2026-10-15.db
```

```text
The audit trail is intact: 214 events, from event 1, at 2026-10-10T02:15:09Z, to event 214, at 2026-10-15T09:41:27Z.
```

It fails, with status 1, if the copy isn't a database, or if its audit trail
is broken.

### Restore a copy

1. Stop `serve`.

2. Move the database out of the way, with any file beside it. A
   `nbpdns.db-wal` file, left by a `serve` that didn't stop cleanly, holds
   the old database's last changes, and doesn't belong to the copy:

   ```shell
   sudo mkdir /var/lib/nbpdns/replaced
   sudo mv /var/lib/nbpdns/nbpdns.db* /var/lib/nbpdns/replaced/
   ```

3. Put the copy in place, with mode 0600, owned by the user that runs
   `serve`: `nbpdns` here, or `65532` in a container.

   ```shell
   sudo install -m 0600 -o nbpdns -g nbpdns /var/backups/nbpdns/nbpdns-2026-10-15.db /var/lib/nbpdns/nbpdns.db
   ```

4. Start `serve`. It applies any migration that the copy lacks, and serves
   the copy's last reports until its first refresh.

The restore loses what changed after the copy: settings, server groups,
audit events, and drift history. `nbpdns audit verify` still passes, since
the trail ends where the copy's did, and `serve`'s start follows it. Keep
the replaced database for as long as you might need what it held.

## Back up a PostgreSQL database

### Before you start

You need:

- root on a host that reaches the database, through `sudo`;
- `pg_dump` and `pg_restore` from PostgreSQL 18, or later, such as from
  Debian's `postgresql-client-18` package;
- `psql`, as a role that can create databases, such as `postgres`, to check
  and restore a copy;
- `database.url`, without its password, and a password file that holds the
  password, which [Run nbpdns on PostgreSQL](run-nbpdns-on-postgresql.md#give-nbpdns-the-password)
  describes. The PostgreSQL tools read the same URL as nbpdns. This guide
  uses:

  ```text
  postgres://nbpdns@db.example.com/nbpdns?sslmode=verify-full&sslrootcert=/etc/nbpdns/pg-ca.pem&passfile=/etc/nbpdns/secrets/pgpass
  ```

Don't put the password in a URL on a command line, where other users of the
host can see it in the process list.

### Copy the database

1. Make a directory for the copies that only root can read. A copy holds the
   audit trail, the drift history, and the stored secrets, sealed:

   ```shell
   sudo install -d -m 0700 /var/backups/nbpdns
   ```

2. Copy the database with `pg_dump`, as nbpdns's role, which owns its
   tables, in its custom format, which `pg_restore` reads:

   ```shell
   sudo pg_dump --format=custom --file=/var/backups/nbpdns/nbpdns-2026-10-15.dump \
     "postgres://nbpdns@db.example.com/nbpdns?sslmode=verify-full&sslrootcert=/etc/nbpdns/pg-ca.pem&passfile=/etc/nbpdns/secrets/pgpass"
   ```

   `pg_dump` reads one snapshot of the database, so the copy holds every
   change committed before it started, while `serve` carries on.

### Check the copy

1. Make a database for the check, owned by nbpdns's role:

   ```shell
   psql -h db.example.com -U postgres -c 'CREATE DATABASE nbpdns_check OWNER nbpdns'
   ```

2. Restore the copy into it, as nbpdns's role:

   ```shell
   sudo pg_restore --dbname="postgres://nbpdns@db.example.com/nbpdns_check?sslmode=verify-full&sslrootcert=/etc/nbpdns/pg-ca.pem&passfile=/etc/nbpdns/secrets/pgpass" \
     /var/backups/nbpdns/nbpdns-2026-10-15.dump
   ```

3. Verify its audit trail, which reads every event in it:

   ```shell
   sudo nbpdns audit verify \
     --database-url "postgres://nbpdns@db.example.com/nbpdns_check?sslmode=verify-full&sslrootcert=/etc/nbpdns/pg-ca.pem&passfile=/etc/nbpdns/secrets/pgpass"
   ```

   ```text
   The audit trail is intact: 214 events, from event 1, at 2026-10-10T02:15:09Z, to event 214, at 2026-10-15T09:41:27Z.
   ```

4. Drop the database you checked it in:

   ```shell
   psql -h db.example.com -U postgres -c 'DROP DATABASE nbpdns_check'
   ```

### Restore a copy

1. Stop `serve`.

2. Rename the database, to keep it while you restore:

   ```shell
   psql -h db.example.com -U postgres -c 'ALTER DATABASE nbpdns RENAME TO nbpdns_replaced'
   ```

3. Make a new, empty database, as the first one was: owned by nbpdns's
   role, which alone can connect to it:

   ```shell
   psql -h db.example.com -U postgres -c 'CREATE DATABASE nbpdns OWNER nbpdns'
   psql -h db.example.com -U postgres -c 'REVOKE CONNECT ON DATABASE nbpdns FROM PUBLIC'
   ```

4. Restore the copy into it, as nbpdns's role:

   ```shell
   sudo pg_restore --dbname="postgres://nbpdns@db.example.com/nbpdns?sslmode=verify-full&sslrootcert=/etc/nbpdns/pg-ca.pem&passfile=/etc/nbpdns/secrets/pgpass" \
     /var/backups/nbpdns/nbpdns-2026-10-15.dump
   ```

5. Start `serve`. It applies any migration that the copy lacks, and serves
   the copy's last reports until its first refresh.

As with SQLite, the restore loses what changed after the copy, and the
audit trail still verifies. Drop `nbpdns_replaced` once you no longer need
what it held.

## Keep the master key apart

A copy holds the stored secrets sealed with the master key,
`database.master_key`. Keep the key away from the copies, since anyone with
both can open the secrets. But keep it: without it, a restored copy's
secrets can't be opened, and `serve` refuses to start.

A copy needs the master key that sealed it. After you rotate the master key,
keep the old one for as long as you keep copies made before the rotation.
[Protect stored secrets with a master key](protect-stored-secrets-with-a-master-key.md)
rotates it.

## Next steps

- [Check the audit trail](check-the-audit-trail.md) reads the events.
- The [command-line reference](../reference/command-line.md#nbpdns-db)
  describes `nbpdns db`, which shows the database's migrations.
