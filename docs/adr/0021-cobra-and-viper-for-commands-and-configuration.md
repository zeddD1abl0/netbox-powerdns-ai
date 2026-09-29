---
title: "0021: Cobra and Viper for commands and configuration"
status: accepted
date: 2026-09-29
decision-makers: [jordan]
requirements: [REQ-002, REQ-005, REQ-006]
questions: []
supersedes:
---

# 0021: Cobra and Viper for commands and configuration

## Context and problem statement

nbpdns is one binary with many commands (REQ-002). Each command needs help,
and the shells need completion. Its settings come from defaults, a config
file, environment variables and flags, in that order of precedence (REQ-005,
REQ-006, `CLAUDE.md`), and every secret needs a `_FILE` variant. Each key is
declared once and its reference is generated (`CLAUDE.md`, principle 2).

`CLAUDE.md` says stdlib first, and a significant dependency gets an ADR. The
stdlib has `flag`, but no subcommands, completion or YAML. In the M01 design
on 2026-09-27, the user chose established libraries over modules of our own:
Cobra and Viper.

## Decision drivers

- Subcommands, help and shell completion that users already know.
- The precedence rule, from a library that's widely used and maintained.
- Every key declared once, driving its flag, variable, file key, validation
  and reference.
- Few dependencies, all with allowed licenses (MIT, BSD, Apache-2.0,
  MPL-2.0).

## Considered options

1. The stdlib `flag` package, with a config loader of our own.
2. Cobra and Viper.
3. urfave/cli with koanf.
4. Kong.

## Decision outcome

Chosen option: **Cobra (Apache-2.0) for commands, and Viper (MIT) for
configuration**, the user's choice, with a thin key registry of our own on
top (`internal/config`).

- **Cobra** gives the command tree, `--help`, usage errors and shell
  completion.
- **Viper** reads the YAML config file, binds each key's environment variable
  and flag, and resolves precedence: flags, then the environment, then the
  file, then the defaults. nbpdns uses a Viper instance per load, never
  Viper's global one.
- **The registry** declares each key once: its name, description, default,
  type and secret flag. From that declaration come:
  - the key's environment variable (`netbox.url` gives `NBPDNS_NETBOX_URL`)
    and flag (`--netbox-url`);
  - **secret files:** a secret also gets `_FILE` variants
    (`NBPDNS_NETBOX_TOKEN_FILE`, `--netbox-token-file`,
    `netbox.token_file`), which count at the same level as the plain form.
    Setting both at one level is an error;
  - **strict keys:** an unknown key in the config file, or an unknown
    `NBPDNS_` variable, is an error;
  - **validation:** each key parses its value with its own rules, whatever
    the source, and every problem is reported at once, naming the key and
    the source;
  - **sources:** `nbpdns config show` prints each value and the flag,
    variable, file or default it came from;
  - **redaction:** `config.Secret` redacts itself in `String`, `Format`,
    JSON, text encoders and `log/slog`. Only `Reveal` returns the value;
  - **the reference:** `make generate` writes
    `docs/reference/configuration.md`, and `make generate-check` fails when
    it's out of date.
- **The command-line reference** is generated from Cobra's command tree, as
  `docs/reference/command-line.md`.

**Two changes from M01's approved design**, found while building it:
- The command-line reference comes from a small generator of our own, not
  `cobra/doc`. `cobra/doc` writes one page per command, starting at a
  level-two heading, with Cobra's own wording. That wording fails the
  documentation style checks, and the pages would need editing after every
  generation. Our generator writes one page in the documentation style, and
  saves two dependencies (go-md2man and blackfriday).
- Unknown config file keys are found by comparing the file's keys with the
  registry, not with Viper's `UnmarshalExact`. The registry parses each key
  itself, so a value from any source is checked by the same rules. For the
  same reason, flags keep their arguments as text, and the registry parses
  them. Viper's own conversions would turn an invalid number into 0 without
  an error.

### Dependency cost

The binary links these modules, all with allowed licenses:

| Module | License | Brought in by |
|---|---|---|
| `github.com/spf13/cobra` | Apache-2.0 | nbpdns |
| `github.com/spf13/pflag` | BSD-3-Clause | Cobra, Viper |
| `github.com/spf13/viper` | MIT | nbpdns |
| `github.com/spf13/afero` | Apache-2.0 | Viper |
| `github.com/spf13/cast` | MIT | Viper |
| `github.com/fsnotify/fsnotify` | BSD-3-Clause | Viper |
| `github.com/go-viper/mapstructure/v2` | MIT | Viper |
| `github.com/pelletier/go-toml/v2` | MIT | Viper |
| `github.com/sagikazarmark/locafero` | MIT | Viper |
| `github.com/subosito/gotenv` | MIT | Viper |
| `go.yaml.in/yaml/v3` | MIT and Apache-2.0 | Viper |
| `golang.org/x/sys` | BSD-3-Clause | fsnotify, afero |
| `golang.org/x/text` | BSD-3-Clause | afero |

`github.com/inconshreveable/mousetrap` (Apache-2.0) is also required, but
Cobra links it only on Windows. The static binary is about 9 MB.

### Consequences

- Good: familiar help and completion, and precedence handled by a
  maintained library.
- Good: one declaration per key drives its flag, variable, file key,
  validation, `config show` and reference, so they can't drift apart.
- Bad: thirteen modules to keep current. `make vuln` runs govulncheck on
  every pipeline. Viper brings in code nbpdns doesn't use, such as TOML,
  dotenv and file watching.
- Bad: Viper treats keys as case-insensitive, and an empty environment
  variable as unset. The configuration reference says so.
- Bad: the registry parses values itself rather than trusting Viper's
  conversions, so it's code to maintain, covered by table-driven tests.

### Confirmation

- `make generate-check`, in `make check` and CI, fails when a generated
  reference page is out of date.
- Table-driven tests in `internal/config` cover precedence, secret files,
  strict keys, validation, sources and redaction.
- `make vuln` scans every linked module.

## Pros and cons of the options

### The stdlib `flag` package with our own loader

- Good: no dependency for flags.
- Bad: no subcommands, help per command or completion. YAML still needs a
  dependency, and precedence and binding are ours to write and maintain.

### urfave/cli with koanf

- Good: koanf is lighter than Viper, with fewer dependencies.
- Bad: not the user's choice, and less widely used than Cobra and Viper.

### Kong

- Good: flags declared as struct tags, with little code.
- Bad: configuration files and precedence need more of our own code.

## More information

- Recorded by ITEM-0020.
- The keys and their rules: `docs/reference/configuration.md`.
