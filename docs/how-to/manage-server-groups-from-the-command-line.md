---
title: Manage server groups from the command line
weight: 39
---

# Manage server groups from the command line

A **server group** is a set of PowerDNS servers that nbpdns compares with
NetBox through one of them, its primary. The config file declares groups
under `powerdns.groups`, and `nbpdns groups` adds, changes, renames and
removes more, which `nbpdns serve` applies within seconds, without a
restart. This guide adds a group, changes it, renames it, and removes it.

Each group has an owner, its `managed_by`:

- `file`: the config file declares it. Serve mirrors the file's groups
  into its database at each start, and `nbpdns groups` can't change them.
- `cli`: `nbpdns groups add` added it, and only `nbpdns groups` changes it.

## Before you start

You need:

- `nbpdns serve` running, with its database at `database.url`;
- a shell as the user that runs `nbpdns serve`, with the same
  `database.url`. In a container, run the commands in the service's own
  container, such as with `docker exec nbpdns`;
- a master key, `database.master_key`, which seals each group's API key in
  the database. [Protect stored secrets with a master key](protect-stored-secrets-with-a-master-key.md)
  makes one;
- the group's primary, with its PowerDNS API on, and its API key in a file,
  as [Connect nbpdns to PowerDNS](connect-nbpdns-to-powerdns.md) describes.

## List the groups

```shell
nbpdns groups list
```

```text
NAME    MANAGED BY  PRIMARY                          VIEWS                DRIFT POLICY  UPDATED               BY
site-a  file        https://pdns-a.example.com:8443  _default_,internal   report        2026-10-14T02:00:00Z  file:/etc/nbpdns/nbpdns.yaml
```

The config file's groups are as serve last mirrored them, when it started.
`nbpdns groups list -o json` shows every field.

## Add a group

1. Add it, with its primary's URL, the NetBox views whose zones it serves,
   and its API key:

   ```shell
   nbpdns groups add site-b --url https://pdns-b.example.com:8443 \
     --views _default_ --api-key-file /etc/nbpdns/secrets/pdns-b-key \
     --reason "the new data center"
   ```

   ```text
   Added server group site-b, its API key sealed with the master key.
   A running nbpdns serve compares it within 5s.
   ```

   nbpdns checks the fields as it would the config file's, then checks that
   the primary answers, as `nbpdns powerdns check` does. If a check fails,
   it shows the checks and adds nothing.

   The key is read from the file that `--api-key-file` names, or from
   standard input with `--api-key-file -`. Without the flag, it's read from
   the environment variable `NBPDNS_GROUP_API_KEY`. It's never taken from
   the command line, where other users can see it.

2. Check that serve compares it. Serve logs `applied the changed server
   groups` and starts a refresh at once, and the API lists the group:

   ```shell
   curl -s http://localhost:8080/api/server-groups/site-b | jq '{name, managed_by, status}'
   ```

   ```json
   {
     "name": "site-b",
     "managed_by": "cli",
     "status": "ok"
   }
   ```

To add a group whose primary isn't up yet, add `--no-check`. The group's
audit event records that the check was skipped, and serve reports the group
as failed until its primary answers.

Other flags set the group's other fields: `--drift-policy`,
`--zone-policies`, `--server-id`, and `--ca-file`, `--cert-file` and
`--key-file` for TLS. [Command line](../reference/command-line.md#nbpdns-groups-add)
lists them.

## Change a group

Each flag given replaces its field. An empty value returns the field to its
default, or removes it:

```shell
nbpdns groups set site-b --views _default_,internal --zone-policies "" --reason "serves internal too"
```

```text
Changed views, zone_policies of server group site-b.
A running nbpdns serve applies it within 5s.
```

A change to how nbpdns reaches the primary, such as its URL, its TLS files
or its API key, is checked first, as an addition is. Other changes aren't.
To change the key, give the new one as `nbpdns groups add` takes it:

```shell
nbpdns groups set site-b --api-key-file /etc/nbpdns/secrets/pdns-b-key.new --reason "key rotated"
```

> [!NOTE]
> `nbpdns groups set` reads the key from `NBPDNS_GROUP_API_KEY` too, when
> `--api-key-file` isn't given. Unset the variable once the group is added,
> so that a later `set` doesn't change the key with it.

## Rename a group

```shell
nbpdns groups rename site-b site-dc2 --reason "named for the site"
```

```text
Renamed server group site-b to site-dc2, with its drift history.
A running nbpdns serve applies it within 5s.
```

The group's drift history and its last report move to the new name, so its
zones carry on where they were. Some things keep the old name:

- **The audit trail:** its events can't be changed, so those before the
  rename name `site-b`. The event of the rename, `group.renamed`, links
  the two names.
- **Dashboards and alerts:** the metrics' `group` label takes the new name
  at once, and the old name's series go. Update any query that names the
  old one.

A name that another group has, or that a removed group's drift history
still uses, is refused. To rename a group that the config file declares,
rename it in the file and restart serve: that's a removal and an addition,
and the old name's history stays under the old name until it's pruned.

## Remove a group

```shell
nbpdns groups remove site-dc2 --reason "data center closed"
```

```text
Removed server group site-dc2. Its drift history is kept until drift.history_retention_days prunes it.
A running nbpdns serve stops comparing it within 5s.
```

Serve drops the group's state and metrics. Its drift history stays, and
`nbpdns drift history --group site-dc2` still reads it.

## Move a group from the config file to the command line

A group's drift history belongs to its name, so a group added under a
removed group's name carries on its history:

1. Remove the group from the config file, and restart serve. Serve removes
   the group from its database, and stops comparing it.
2. Add it with `nbpdns groups add`, under the same name, with the same
   fields.

Moving it back to the file is the reverse: remove it with
`nbpdns groups remove`, then declare it in the file and restart serve.

> [!WARNING]
> A name that both the config file and `nbpdns groups` claim stops serve at
> its next start, with an error that names the group. `nbpdns groups add`
> refuses a name that its own config file declares, but another host's
> config file may differ.

## See who changed what

Each change is an audit event, with the operating system's user who made
it, and the reason. Serve's mirror of the config file records its changes
too, with the file as the actor:

```shell
nbpdns audit list --target server-group
```

```text
SEQ  TIME                  ACTOR           ACTION         TARGET                   OUTCOME  REASON
402  2026-10-14T03:05:11Z  os-user:jordan  group.created  server-group:site-b      success  the new data center
405  2026-10-14T03:20:40Z  os-user:jordan  group.changed  server-group:site-b      success  serves internal too
409  2026-10-14T04:02:13Z  os-user:jordan  group.renamed  server-group:site-dc2    success  named for the site
```

[Check the audit trail](check-the-audit-trail.md) shows how to read each
event in full: its fields before and after the change.

## Next steps

- [Command line](../reference/command-line.md#nbpdns-groups) describes
  `nbpdns groups`.
- [Audit events](../reference/audit-events.md) describes the `group.*`
  events.
- [Configuration](../reference/configuration.md#powerdnsgroups) describes
  a group's fields.
