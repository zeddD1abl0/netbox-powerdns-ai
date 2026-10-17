---
title: Configuration sources and precedence
weight: 20
---

# Configuration sources and precedence

nbpdns takes each setting from the first of these that sets it: a flag, an
`NBPDNS_` environment variable, the YAML config file, the database (for a
runtime key), or the setting's default. This page explains that order, which
settings can be in the database, why nbpdns rejects names it doesn't know,
and how it handles secrets. The
[configuration reference](../reference/configuration.md) lists every setting.

## Why this order

The more specific a source is, the later it applies:

- a **default** suits most installations;
- the **database** holds the settings changed while nbpdns runs, with
  `nbpdns settings set`;
- the **config file** describes one installation, and changes when the
  installation does;
- the **environment** describes one process or container, and is how
  container platforms such as Kubernetes pass settings in;
- a **flag** describes one run, such as an operator adding `--log-level debug`
  while looking into a problem.

So a container can take most of its settings from a file baked into its
image, override a few from its environment, and still be run once by hand with
a flag. Most command-line tools use the same order.

## Runtime and bootstrap keys

Each setting's key is a runtime key or a bootstrap key, and the
[configuration reference](../reference/configuration.md) shows which.

- A **runtime** key, such as `drift.interval`, is one that `nbpdns serve`
  can apply while it runs. `nbpdns settings set` stores its value in the
  database, and a running `serve` uses it within seconds.
- A **bootstrap** key, such as `database.url` or `netbox.url`, can't change
  while `serve` runs, or shouldn't change from where settings are changed.
  It comes only from a flag, the environment, or the config file, and takes
  effect when `serve` starts.

A runtime key that a flag, the environment, or the config file sets is
**pinned**: that value wins, and the database's value is kept but not used,
until the key is unset there and `serve` restarts. `nbpdns settings` says
which keys are pinned. The database comes below the file because the file,
the environment and the flags are how an installation is declared, often by
a deployment tool, and a change made while nbpdns runs shouldn't override
what was declared.
[How nbpdns stores its state](how-nbpdns-stores-its-state.md#settings-in-the-database)
explains why each key is the kind it is, and
[Change nbpdns's settings while it runs](../how-to/change-nbpdns-settings-while-it-runs.md)
changes one.

## Each setting is declared once

Each setting is declared once in nbpdns's code, with its name, type, default,
and description. That one declaration gives the setting its environment
variable, its flag, and its place in the config file. For example, the setting
`netbox.url` is:

- `NBPDNS_NETBOX_URL` in the environment;
- `--netbox-url` on the command line;
- `url` under `netbox` in the config file.

The configuration and command-line reference pages are generated from the
same declarations, so the documentation can't drift from the code. nbpdns
uses [Cobra and Viper](../adr/0021-cobra-and-viper-for-commands-and-configuration.md)
for its commands and for reading the sources, with this registry on top.

## Resources that the config file declares

Some configuration isn't a single setting but a list of resources, each with
fields of its own. The PowerDNS server groups are one: each group has a name,
the NetBox views it serves, and its primary's URL and API key
([ADR-0026](../adr/0026-read-powerdns-through-its-api-with-powerdns-5-1-on.md)).
A list like that doesn't map onto environment variables and flags without an
invented naming scheme, so the config file declares it, under
`powerdns.groups`, and the environment and flags can't.

The same rules apply to its fields: each is declared once in the code, which
also generates their reference; an unknown field is an error; an API key can
come from a file, with `primary.api_key_file`; and `nbpdns config show` lists
each field of each group, with the key redacted.

`nbpdns serve` mirrors the file's groups into its database at each start,
each managed by the file, and `nbpdns groups` adds more, managed by the
command line, which serve applies while it runs
([ADR-0040](../adr/0040-runtime-settings-and-managed-resources.md)). Each
group has one owner: a name that both claim stops serve.

## Unknown names are errors

A misspelled setting would otherwise be ignored without a word, and its
default used instead: `NBPDNS_NETBOX_TIMOUT=5m` would leave the timeout at
30 seconds. So nbpdns treats a config file key or an `NBPDNS_` variable that
isn't a setting as an error. It checks every source before it stops, and
reports every problem it finds, so they can all be fixed at once.

An empty environment variable counts as unset, so `NBPDNS_LOG_LEVEL=` doesn't
hide the config file's value.

## Secrets

A secret, such as `netbox.token`, never appears in nbpdns's output or logs.
It shows as `[redacted]` in `nbpdns config show`, and in any log line or JSON
that would otherwise contain it.

A secret that the database stores, such as a `netbox.token` that
`nbpdns settings set` stored, is sealed with the master key,
`database.master_key`, and shows as `[redacted]` too. [How nbpdns stores its
state](how-nbpdns-stores-its-state.md#secrets-at-rest) explains how.

Every secret can also be read from a file, named by its `_file` key,
`_FILE` variable, or `-file` flag. That's how container platforms and systemd
hand secrets to a service. It also keeps the secret out of the environment,
which child processes inherit, and out of the command line, which other
users of the host can see in the process list.

The file form counts at the same level as the plain form: a token file named
in the environment overrides a token in the config file. Setting both forms at
the same level is an error, since nbpdns couldn't know which one you meant.

## Where a value came from

With five sources, it's not always clear which one won. `nbpdns config show`
lists each setting's value and where it came from: the flag, the environment
variable, the config file, the database, or the default. For a pinned key,
it names the source that wins, and `nbpdns settings list` shows the value
that the database keeps.
