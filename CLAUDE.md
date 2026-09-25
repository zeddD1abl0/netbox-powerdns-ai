# CLAUDE.md

Claude develops this project end to end. This file holds the working rules and
tells you where everything else lives. It never holds project state: status
lives in `project/`, and decisions live in `docs/adr/`.

## Start of every session

1. Read [`project/README.md`](project/README.md), the board. It shows the
   current milestone, the open items and the open questions.
2. Read the current milestone's file in [`project/milestones/`](project/milestones/).
3. Read only the items, ADRs and docs the task needs. Don't read whole
   directories.

## Where things live

| What | Where |
|---|---|
| The original brief and dated answers (append-only) | [`project/brief.md`](project/brief.md) |
| Requirements (`REQ-nnn`) and open questions (`Q-nnn`) | [`project/requirements.md`](project/requirements.md) |
| Milestones: goal, non-goals, approved design, acceptance, verification log | `project/milestones/Mnn-*.md` |
| Work items: features, bugs, debt, tasks | `project/items/ITEM-nnnn-*.md` |
| Decisions and their rationale | `docs/adr/nnnn-*.md` |
| Product documentation (Diátaxis) | `docs/tutorials/`, `docs/how-to/`, `docs/reference/`, `docs/explanation/` |
| How to write the docs | [`docs/contributing/documentation-style.md`](docs/contributing/documentation-style.md) |
| API contract (from M1) | `api/openapi.yaml` |

## Workflow

- Design each milestone in plan mode: ask the open questions, then write the
  plan. Once the user approves it, copy the approved plan verbatim into the
  milestone file under **Approved design**.
- Implement in auto mode. When the implementation is done and verified, return
  to plan mode.
- **Commits** ([ADR-0010](docs/adr/0010-claude-commits-per-item-on-milestone-branches.md)):
  - Work on the milestone branch `mNN-short-title`, created from `main` when
    the milestone starts.
  - Commit when each item is done. Checkpoint commits are fine.
  - Use Conventional Commits, with a `Refs: ITEM-nnnn` trailer on every commit.
  - Never commit to `main`, merge into `main`, or push. A hook blocks commits on
    `main`.
  - The user reviews the branch, merges it (not a squash merge) and pushes.
- Don't implement milestone N until milestone N-1 is **complete** (every item
  done, `make check` green) **and merged** (its branch is in `main`). If asked
  to start early, warn and list what's outstanding. The user may override.

## Tracking rules

- Every piece of work has an item. Create one with the `new-item` skill before
  starting.
- Status lives in front matter: `open` → `in-progress` → `done`, or `blocked` or
  `wontfix`. Set `closed:` when an item is done or dropped.
- **Files never move or get renamed.** Closing an item changes its status only.
- Item notes are append-only and dated (`YYYY-MM-DD`).
- A decision with lasting consequences gets an ADR (`new-adr` skill). Accepted
  ADRs aren't edited: a new ADR supersedes them.
- A problem found while working becomes an item, not a code comment or a TODO.
- When a question is answered, move it from **Open questions** to **Answered**
  in `project/requirements.md` with the date. Link the ADR or REQ it produced.
- `project/README.md` is generated. Until `projctl` exists (ITEM-0005), update it
  by hand in the same change as the items it lists.

## Principles

1. **Self-contained and not tied to a forge** ([ADR-0003](docs/adr/0003-self-contained-forge-neutral-toolchain.md)).
   The only prerequisites are Go, Docker and make. Pin every tool in the repo.
   CI files only call make targets. Vendor UI assets; no CDNs. Don't rely on
   forge features (issues, wiki, Pages).
2. **One source, generated references.** Config keys, metrics, audit events and
   permissions are declared once in code, and their reference docs are
   generated. Never hand-edit a generated file.
3. **Spec first.** API changes start in `api/openapi.yaml`. Server code is
   generated from it and responses are contract-tested against it.
4. **Docs ship with code.** A change isn't done until its docs, generated
   references and CHANGELOG entry are in the same change.
5. **Small files that never move.** One file per item, ADR and milestone.

## Engineering standards

The full rationale lives in ADRs. Where no ADR exists yet, these are the
defaults.

- **Go:** Google Go Style Guide. Code goes under `internal/`. Stdlib first.
  Justify each new dependency in its item; a significant one gets an ADR.
  Allowed licenses: MIT, BSD, Apache-2.0, MPL-2.0.
- **API:** OpenAPI 3.1 and the Zalando RESTful API Guidelines. Errors use RFC
  9457 problem+json. Paths are versioned as `/api/v1`. Updates use
  ETag/If-Match, lists use cursor pagination, and creates accept an
  `Idempotency-Key`.
- **Config:** precedence is defaults < file < env < flags. Runtime settings live
  in the DB unless env or file pins them. Bootstrap keys are never editable from
  the web. Every secret has a `*_FILE` variant.
- **Logs:** `log/slog` JSON, with `trace_id` and `request_id` on every line.
  Never log a secret; wrap secrets in the redacting type. The audit stream is
  separate from the operational log.
- **Metrics and tracing:** Prometheus naming best practice. OpenTelemetry traces
  with W3C `traceparent`, propagated into NetBox and PowerDNS calls.
- **Audit:** every state change emits an audit event: actor, action, target,
  before and after values, reason, `trace_id`, source IP and outcome.
- **Tests:** table-driven, always run with `-race`. Integration tests run
  against real PowerDNS and NetBox in containers, never mocks of their APIs.
- **Commits:** Conventional Commits. User-facing changes get a line in
  `CHANGELOG.md` under `Unreleased`.

## Documentation rules

- Follow [`docs/contributing/documentation-style.md`](docs/contributing/documentation-style.md).
- Write plain Markdown that reads well raw on GitHub and GitLab. Use GitHub
  alerts (`> [!NOTE]`) for callouts and Mermaid code blocks for diagrams.
- Put each page in exactly one Diátaxis section.

## Commands

The toolchain arrives in M0 (ITEM-0004 to ITEM-0008). Until then there are no
build commands. The planned targets:

| Command | Does |
|---|---|
| `make check` | Everything CI checks: fmt, lint, vet, tests with `-race`, vuln scan, OpenAPI lint, docs lint, `projctl lint` |
| `make ci` | The full CI pipeline, run locally |
| `make test-integration` | Integration tests against containerised NetBox and PowerDNS |
| `make docs` | Build the documentation site |
| `make project` | Regenerate `project/README.md` |

## Definition of Done

**An item** is done when:
- its code and tests are written;
- `make check` is green;
- generated references are regenerated and committed;
- the relevant docs page is updated;
- `CHANGELOG.md` has a line, if it's user-facing;
- its status is `done` with a `closed:` date;
- the board is updated;
- it's committed on the milestone branch with its `Refs` trailer.

**A milestone** is done when:
- every item in it is done;
- `/code-review high` has run, plus `/security-review` if auth, audit or secrets
  changed;
- the manual verification steps are recorded in the milestone file;
- the user has merged its branch into `main`.

Use the `close-milestone` skill.
