---
title: Configure nbpdns
weight: 30
---

# Configure nbpdns

nbpdns takes its settings from a YAML config file, `NBPDNS_` environment
variables, and command-line flags. This guide sets up a config file, keeps
the secrets in files, overrides a setting for one run, and confirms the
result.

Every setting, with its variable, flag, and default, is in the
[configuration reference](../reference/configuration.md).
[Configuration sources and precedence](../explanation/configuration-sources-and-precedence.md)
explains how the sources combine.

## Write a config file

1. Create a YAML file, such as `/etc/nbpdns/nbpdns.yaml`, with the settings
   you want to fix. Leave out anything you're happy with the default for.

   ```yaml
   log:
     level: info
     format: json
   netbox:
     url: https://netbox.example.com
     token_file: /etc/nbpdns/netbox-token
     timeout: 30s
   ```

2. Tell nbpdns where the file is, with `--config` or `NBPDNS_CONFIG`:

   ```shell
   export NBPDNS_CONFIG=/etc/nbpdns/nbpdns.yaml
   ```

   nbpdns reads no config file unless you name one.

An unknown key in the file is an error, so a misspelled key can't be ignored
silently.

## Keep secrets in files

A secret, such as `netbox.token`, can be set directly, or read from a file
named by its `_file` form. Use the file:

- in the config file, `token_file` under `netbox`;
- in the environment, `NBPDNS_NETBOX_TOKEN_FILE`;
- on the command line, `--netbox-token-file`.

The file holds only the secret. A final newline is ignored. Make it readable
only by the user nbpdns runs as.

Setting both forms of a secret in one place, such as `NBPDNS_NETBOX_TOKEN` and
`NBPDNS_NETBOX_TOKEN_FILE`, is an error.

## Override a setting

An environment variable overrides the config file, and a flag overrides both.
For one run with debug logs:

```shell
nbpdns netbox zones --log-level debug
```

Or for everything run from this shell:

```shell
export NBPDNS_LOG_LEVEL=debug
```

An empty environment variable counts as unset. An `NBPDNS_` variable that
isn't a setting is an error.

## Confirm the result

```shell
nbpdns config show
```

```text
KEY                 VALUE                       SOURCE
log.format          json                        file /etc/nbpdns/nbpdns.yaml
log.level           debug                       env NBPDNS_LOG_LEVEL
netbox.ca_file                                  default
netbox.concurrency  4                           default
netbox.page_size    500                         default
netbox.timeout      30s                         file /etc/nbpdns/nbpdns.yaml
netbox.token        [redacted]                  file /etc/nbpdns/nbpdns.yaml
netbox.url          https://netbox.example.com  file /etc/nbpdns/nbpdns.yaml
```

Each setting shows its value and where it came from. Secrets show as
`[redacted]`. Add `--output json` for a form that scripts can read.

If any setting is wrong, `config show` lists every problem and exits with
status 1, so you can fix them all at once.

## Trust a private certificate authority

If NetBox's certificate comes from a private certificate authority, give
nbpdns the authority's certificate as a PEM file:

```yaml
netbox:
  ca_file: /etc/nbpdns/netbox-ca.pem
```

nbpdns trusts that file as well as the system's certificate authorities.
There's no setting that turns certificate checks off.
