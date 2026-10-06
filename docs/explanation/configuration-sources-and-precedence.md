---
title: Configuration sources and precedence
weight: 20
---

# Configuration sources and precedence

nbpdns takes each setting from the first of these that sets it: a flag, an
`NBPDNS_` environment variable, the YAML config file, or the setting's
default. This page explains that order, why nbpdns rejects names it doesn't
know, and how it handles secrets. The
[configuration reference](../reference/configuration.md) lists every setting.

## Why this order

The more specific a source is, the later it applies:

- a **default** suits most installations;
- the **config file** describes one installation, and changes when the
  installation does;
- the **environment** describes one process or container, and is how
  container platforms such as Kubernetes pass settings in;
- a **flag** describes one run, such as an operator adding `--log-level debug`
  while looking into a problem.

So a container can take most of its settings from a file baked into its
image, override a few from its environment, and still be run once by hand with
a flag. Most command-line tools use the same order.

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

Every secret can also be read from a file, named by its `_file` key,
`_FILE` variable, or `-file` flag. That's how container platforms and systemd
hand secrets to a service. It also keeps the secret out of the environment,
which child processes inherit, and out of the command line, which other
users of the host can see in the process list.

The file form counts at the same level as the plain form: a token file named
in the environment overrides a token in the config file. Setting both forms at
the same level is an error, since nbpdns couldn't know which one you meant.

## Where a value came from

With four sources, it's not always clear which one won. `nbpdns config show`
lists each setting's value and where it came from: the flag, the environment
variable, the config file, or the default.

## Settings that change at runtime

Later releases add settings that can change while nbpdns runs, stored in its
database. A setting that the environment or the config file sets stays fixed:
the more specific source still wins.
