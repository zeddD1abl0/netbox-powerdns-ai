---
id: ITEM-0020
title: Command line, config registry and generated references
type: feature # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M01
requirements: [REQ-001, REQ-002, REQ-005, REQ-006]
depends_on: [ITEM-0019]
created: 2026-09-27
closed: 2026-09-29
---

# ITEM-0020: Command line, config registry and generated references

## Goal

The `nbpdns` binary, with Cobra commands and a Viper-backed config registry
that declares each key once. It generates the configuration and command-line
references.

## Acceptance criteria

- [x] ADR-0021 records Cobra and Viper, their dependency cost, and what the registry adds on top.
- [x] Every module added to `go.mod` is listed here with its license, and each license is MIT, BSD, Apache-2.0 or MPL-2.0.
- [x] `nbpdns version`, `config show` and `completion` work. `make build` produces a static binary in `bin/`.
- [x] Table-driven tests cover precedence (defaults < file < env < flags), `_FILE` secrets at their plain form's level, both forms set, unknown file keys and `NBPDNS_*` variables, source reporting, and `config.Secret` redaction.
- [x] `make generate` writes `docs/reference/configuration.md` and the command-line reference. `make generate-check` is part of `make check`, and fails on a stale page.
- [x] A `build` CI job runs on both forges, and `make project-lint` passes.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-09-27: Created from M01's approved design, before implementation
  started.
- 2026-09-29: Done.
  - **Code:**
    - `cmd/nbpdns` holds the binary.
    - `internal/cli` has the Cobra tree: `version`, `config show`,
      `completion`, `--output table|json`, and exit codes 0, 1 and 2.
    - `internal/config` has the registry, the Viper-backed `Loader` and
      `Secret`.
    - `internal/version` reads the build info.
    - `internal/cmd/gendocs` writes the two reference pages.
  - **Modules added to `go.mod`,** with the license read from each module's
    LICENSE file:
    - `github.com/spf13/cobra` v1.10.2: Apache-2.0
    - `github.com/spf13/viper` v1.21.0: MIT
    - `github.com/spf13/pflag` v1.0.10: BSD-3-Clause
    - `github.com/spf13/afero` v1.15.0: Apache-2.0
    - `github.com/spf13/cast` v1.10.0: MIT
    - `github.com/fsnotify/fsnotify` v1.10.1: BSD-3-Clause
    - `github.com/go-viper/mapstructure/v2` v2.5.0: MIT
    - `github.com/pelletier/go-toml/v2` v2.4.3: MIT
    - `github.com/sagikazarmark/locafero` v0.12.0: MIT
    - `github.com/subosito/gotenv` v1.6.0: MIT
    - `go.yaml.in/yaml/v3` v3.0.5: MIT and Apache-2.0
    - `golang.org/x/sys` v0.48.0: BSD-3-Clause
    - `golang.org/x/text` v0.42.0: BSD-3-Clause
    - `github.com/inconshreveable/mousetrap` v1.1.0: Apache-2.0 (linked
      only on Windows)

    Viper required `x/sys` v0.29.0 and `x/text` v0.28.0, and govulncheck
    found a known vulnerability in each (not reachable from our code). Every
    module was updated to its latest release, and govulncheck now finds
    none.
  - **Two changes from the approved design,** recorded in ADR-0021:
    - the command-line reference comes from a generator of our own, not
      `cobra/doc`, so it's one page that passes the docs style;
    - unknown file keys are found by comparing with the registry, not
      `UnmarshalExact`, because the registry parses every value itself.
  - **Also found:** Cobra doesn't validate a group command's arguments unless
    it can run, so `nbpdns frobnicate` printed help and exited 0. Group
    commands now print their help as their own action, and a stray argument
    exits 2.
  - **Checks:**
    - `make build` gives a statically linked binary of about 9 MB. `go version -m`
      shows `CGO_ENABLED=0` and the VCS stamp.
    - `make generate-check` fails on a hand-edited page, with the diff.
  - **CI:** the `build` job and `generate-check`, in `go-lint`, are defined
    on both forges, and `make project-lint` confirms they mirror `make ci`.
    They first run on the next push.
