---
title: '0037: Plan milestones ahead, with GitLab milestones and issues as their view'
status: accepted # proposed | accepted | rejected | deprecated | superseded by ADR-NNNN
date: 2026-10-09
decision-makers: [jordan]
requirements: [REQ-025, REQ-026]
questions: []
supersedes: # ADR-NNNN this replaces, if any
---

# 0037: Plan milestones ahead, with GitLab milestones and issues as their view

## Context and problem statement

Each milestone is designed when it starts, in plan mode, and its ADRs are
accepted then (ADR-0010, ADR-0019). Until then, a milestone is a stub: a goal
and a provisional scope. The user wants more than that:

- **Future goals people can read:** four or five milestones planned ahead,
  so that their questions can be answered, and their design done, while a
  milestone's pipelines run or its merge waits.
- **GitLab as the view:** milestones and issues on GitLab, for planning and
  for registering work in a normal development flow. On 2026-10-09 the user
  gave Claude's project access token the Planner role with the `api` scope:
  it can manage milestones and issues, and comment, but can't push, open
  merge requests or make tags. Every push, tag and merge stays the user's,
  since pushed code runs on their privileged CI runner.
- **A release for each milestone:** an internal release, tagged, at each
  milestone's merge, for its history, though nothing is to be used until
  the user judges it stable and secure, around M20. `v0.1.0`, from M05, is
  the only release so far.

REQ-026 says the project isn't tied to any forge. The user clarified its
scope on 2026-10-09: it's for the code, the build and the pipeline, which
runs on GitLab and GitHub, and may not always run on GitLab, or on x86_64.
The project uses GitLab for tracking and tracing, so its features may be
used there. GitLab's push mirror copies only the repository to GitHub:
commits, branches and tags, but no milestones, issues or merge requests.

## Decision drivers

- `project/` is the record (ADR-0002, REQ-025): it must stay complete and
  travel with the code, to GitHub or anywhere else.
- Nothing in the code, the build or CI may depend on a forge (REQ-026).
- Every push, tag and merge stays with the user.
- A design written long ahead goes stale: M07 found five things wrong with
  ADR-0035 a day after it was accepted (ADR-0036).
- Versions must be valid semantic versions, which Go's module tooling and
  the version that Go stamps into each build need, sort in order after
  `v0.1.0`, and leave room for a fix between milestones.
- Until it's stable, a version mustn't claim stability, which semantic
  versioning reserves for 1.0 and later.

## Considered options

1. **Planning:** at each milestone's start, as now; ahead, with the ADRs
   kept `proposed`; ahead, with the ADRs accepted.
2. **GitLab's view:** none; files generated for the user to paste; a Planner
   token, with which Claude keeps GitLab in step; a Developer token, with
   which Claude would also push.
3. **Milestone versions:** none until 1.0; `v0.1-mNN`; pre-releases of 1.0,
   `v1.0.0-mNN`; `v0.1.NN`, the milestone as the patch; `v0.NN.0`, the
   milestone as the minor, released each time.

## Decision outcome

Chosen: planning ahead with the ADRs kept `proposed`, a Planner token, and
a `v0.NN.0` release for each milestone. The user chose the token and the
versions on 2026-10-09.

**The record and its view:**
- `project/` stays the source of truth. GitLab's milestones and issues are a
  view of it, and an intake for it. Nothing in the code, the build or CI
  reads them.
- **Milestones:** each `project/milestones/Mnn-*.md` has a GitLab milestone,
  titled `Mnn: Title`, whose description gives the goal and the file's path.
  Claude creates and updates it when the file is written or its goal or
  status changes, and closes it once the milestone's branch is merged.
- **Merge requests:** a description that Claude writes starts with
  `/milestone %"Mnn: Title"`, which assigns the milestone as the merge
  request is created, and lists `Closes #n` for each issue it resolves.
- **Issues:** anyone with access may file one. At the start of a session,
  Claude reads the open issues that no item tracks yet. For each, it makes an
  item, whose notes cite the issue's `#n`, comments on the issue with the
  item's ID and milestone, and sets the issue's milestone. An issue closes
  when the merge request that `Closes` it is merged. Claude closes one
  itself only when the user agrees, such as for a duplicate or won't-fix.

**Planning ahead:**
- Planning happens on a **planning branch**, `plan-mNN-mMM`, from `main`,
  which holds only tracking and docs, and is merged like a milestone's,
  through a merge request with a merge commit. One planning branch is open
  at a time, and it's merged before the next milestone's branch starts.
- On it, each planned milestone's stub becomes a **roadmap entry**: its
  goal, scope, non-goals, dependencies, open questions, and candidate
  decisions. Rewriting a stub there is the re-plan that ADR-0019 requires
  an ADR for; renumbering milestones still needs an ADR of its own.
- Milestones may also be **designed** there, in plan mode with the user, as
  each is now: the questions asked, the plan approved and recorded under
  **Approved design**, and its items made. Its ADRs stay `proposed`.
- **Items:** the planning work is an item of the first milestone it plans.
  Each milestone's design, and the items it makes, are that milestone's.
- **At a milestone's start,** Claude checks its design and ADRs against what
  the milestones before it found, changes them while they're still
  proposals, and asks the user to accept the ADRs. Then the milestone is in
  progress. The milestone gate is unchanged: a milestone's implementation
  starts only once the one before it is merged (ADR-0010).

**Milestone releases:**
- Once a milestone's branch is merged, the user tags the merge commit with
  an annotated `v0.NN.0`, NN being the milestone's number without its
  leading zero, which semantic versioning doesn't allow: `v0.7.0` for M07,
  `v0.10.0` for M10. A fix between milestones is a patch release, such as
  `v0.8.1`.
- The tag's pipeline publishes the release, as ADR-0030 has it: the GitLab
  release, the archives and their checksums, and the image, which moves
  `latest`. They're internal releases, in the private project's registries,
  and none is meant for use before the user calls nbpdns stable.
- When a milestone closes, Claude renames `## [Unreleased]` in the CHANGELOG
  to `## [0.NN.0] - date` on its branch, so that the tagged commit holds the
  release's notes.
- Once the user judges nbpdns stable and secure, milestones are released as
  `v1.NN.0` from then on, the milestone's number staying the minor version.
- M07, merged before this, is released from this planning branch's merge:
  its CHANGELOG section, `[0.7.0]`, is written here, and the user tags the
  merge commit `v0.7.0`. The `v1.0.0-m07` tag, made first, is deleted.

### Consequences

- Good: the roadmap, its open questions, and the work in flight can be read
  on GitLab, and planning can go on while pipelines run.
- Good: `project/` stays complete, so the record survives a move away from
  GitLab, and GitHub has it through the mirror.
- Good: the token can't push or tag, so nothing Claude does reaches the
  runner, or a release, without the user.
- Good: each milestone has a release, with its notes, and builds carry a
  meaningful version.
- Bad: the GitLab view can fall behind the files, if a session doesn't
  update it.
- Bad: designs written ahead may need rework at their milestone's start.
  Keeping their ADRs proposed keeps that cheap, but it's still work.
- Bad: each milestone's release runs a full pipeline on the runner, about 50
  minutes, and its image and archives take room in the registries, though
  none is meant for use.
- Bad: before 1.0, a minor release may break things, as semantic
  versioning allows for 0.x.

### Confirmation

- `make project-lint` keeps checking `project/`, which stays the record.
- The `close-milestone` skill checks the CHANGELOG for a section for
  `0.NN.0`, which `go run ./internal/cmd/releasenotes -tag v0.NN.0` prints,
  and gives the user the tag to make after the merge, and the merge
  request's `/milestone` line.
- Each session's first steps include reading the open issues.

## Pros and cons of the options

### Planning at each milestone's start

- Good: designs use everything learned up to then.
- Bad: no readable roadmap, and design waits for the merge before it.

### Planning ahead, with the ADRs accepted

- Good: decisions are final early.
- Bad: what implementation finds means superseding them, as ADR-0036 did
  ADR-0035.

### Files generated for the user to paste

- Good: no write access at all.
- Bad: the user does every update, and the view falls behind.

### A Developer token

- Good: Claude could push branches and open merge requests too.
- Bad: pushed code would run on the user's privileged runner without the
  user's look first. The user declined it.

### No releases until 1.0

- Good: no release pipelines.
- Bad: no milestone in the history, and builds go on naming `v0.1.0`.

### `v0.1-mNN`

- Good: short.
- Bad: not a semantic version, so Go ignores it; and its nearest valid
  form, `v0.1.0-mNN`, sorts before `v0.1.0`.

### Pre-releases of 1.0, `v1.0.0-mNN`

- Good: publishes nothing, since the release job runs only for plain tags.
- Bad: tools that ask for the latest release skip pre-releases, and the
  user preferred internal releases with plain versions. `v1.0.0-m07` was
  made, then dropped.

### `v0.1.NN`, the milestone as the patch

- Good: plain versions, in order.
- Bad: no room for a fix between milestones, since `v0.1.9` is M09's.

### `v0.NN.0`, the milestone as the minor

- Good: plain versions, in order, with room for patch releases, and 0.x
  means what semantic versioning says: in development.
- Bad: each milestone publishes a release that no one is to use yet.

## More information

- ADR-0002 (tracking in the repository), ADR-0010 (branches and commits),
  ADR-0019 (the milestones), ADR-0030 (releases).
- ITEM-0084 implements this, on `plan-m08-m12`.
