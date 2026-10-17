---
title: Protect stored secrets with a master key
weight: 40
---

# Protect stored secrets with a master key

A secret runtime setting, such as `netbox.token`, can be stored in nbpdns's
database, where `nbpdns serve` reads it, and a change reaches serve without
a restart. So is the API key of each server group that `nbpdns groups add`
adds. nbpdns seals each secret it stores with the **master key**,
`database.master_key`, so that the database file alone gives none of them
away. This guide makes a master key, stores a secret, and rotates the key.

## Before you start

You need:

- `nbpdns serve`'s database, at `database.url`, and a shell as the user that
  runs serve;
- OpenSSL, to make a key.

[Change nbpdns's settings while it runs](change-nbpdns-settings-while-it-runs.md)
explains runtime settings. A secret that the config file, the environment
or a flag sets needs no master key: only the database's are sealed.

## Make a master key

1. Make 32 random bytes, in base64, in a file only nbpdns's user can read:

   ```shell
   (umask 077 && openssl rand -base64 32 > /etc/nbpdns/secrets/master-key)
   ```

2. Name the file in the config file, or in `NBPDNS_DATABASE_MASTER_KEY_FILE`,
   for serve and for the commands you run:

   ```yaml
   database:
     master_key_file: /etc/nbpdns/secrets/master-key
   ```

   In a container, mount it as a secret, as
   [Run nbpdns in a container](run-nbpdns-in-a-container.md) does the
   others.

> [!WARNING]
> Keep the master key apart from the database and its backups, and back it
> up too: without it, the secrets that the database stores are lost.
> Anyone who has both can read them.

## Store a secret

Pass a secret in a file, or on standard input with `--from-file -`, never
on the command line, where other users of the host can see it:

```shell
nbpdns settings set netbox.token --from-file /etc/nbpdns/secrets/netbox-token \
  --reason "nbpdns's new NetBox token"
```

```text
Stored [redacted] for netbox.token, sealed with the master key.
A running nbpdns serve applies it within 5s.
```

nbpdns seals the value with a new data key, which the master key wraps,
both with AES-256-GCM, and binds it to `netbox.token`, so that it can't be
moved to another key. Its audit event shows the value as `[redacted]`.

Serve uses the new token from its next refresh. To check where the token
comes from:

```shell
nbpdns settings list | grep netbox.token
```

```text
netbox.token                  [redacted]  database  [redacted]
```

If serve's environment or config file still sets `netbox.token`, that one
wins: remove it from there, and restart serve.

`nbpdns serve` refuses to start if the database holds a secret that its
master key can't open, and says which key sealed it.

## Rotate the master key

Rotating re-wraps each secret's data key with a new master key. The secrets
themselves don't change.

1. Make the new key, as before, in another file:

   ```shell
   (umask 077 && openssl rand -base64 32 > /etc/nbpdns/secrets/master-key.new)
   ```

2. Stop serve. The rotation refuses while it runs, since serve opens the
   secrets with the old key.

3. Rotate, with `database.master_key` still the old key:

   ```shell
   nbpdns secrets rotate-master-key --new-key-file /etc/nbpdns/secrets/master-key.new \
     --reason "yearly rotation"
   ```

   ```text
   Re-wrapped 1 secret with master key 3f0a9c41e8b27d65.
   Set database.master_key to the new key before nbpdns serve starts again.
   ```

   It re-wraps every secret in one transaction, and records it as a
   `master_key.rotated` audit event. If one doesn't open, it changes
   nothing.

4. Make the new key the master key, and start serve:

   ```shell
   mv /etc/nbpdns/secrets/master-key.new /etc/nbpdns/secrets/master-key
   ```

Keep a copy of the old key until serve has started with the new one.

The rotation re-wraps the server groups' API keys too. From the rotation on,
`nbpdns settings set` and `nbpdns groups` refuse a secret sealed with the
old key, even one sealed while the rotation ran, so that serve can open every
secret with the new key. Give the new key to each host that stores secrets.

## If the master key is lost

The secrets it sealed can't be opened. Remove each one, which needs no key,
and store it again with a new master key:

```shell
nbpdns settings unset netbox.token --reason "master key lost"
```

A server group's key goes with its group: remove the group with
`nbpdns groups remove`, and add it again, as
[Manage server groups from the command line](manage-server-groups-from-the-command-line.md)
shows.

The new key can store a secret only once none sealed with the lost key is
left.

## Next steps

- [How nbpdns stores its state](../explanation/how-nbpdns-stores-its-state.md#secrets-at-rest)
  explains how secrets are sealed.
- [Configuration](../reference/configuration.md#databasemaster_key)
  describes `database.master_key`.
- [Check the audit trail](check-the-audit-trail.md) reads the events of
  each change.
