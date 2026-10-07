---
title: "0031: Debian images for CI, with a distroless static runtime image"
status: accepted
date: 2026-10-07
decision-makers: [jordan]
requirements: [REQ-003, REQ-038, REQ-045]
questions: [Q-025]
supersedes: ADR-0015
---

# 0031: Debian images for CI, with a distroless static runtime image

## Context and problem statement

ADR-0015 chose glibc-based Debian or Ubuntu images over Alpine for CI and
for container builds. It left the runtime image to be chosen with the
packaging (Q-025), and expected a glibc variant of distroless.

M05 builds the runtime image. nbpdns is built with `CGO_ENABLED=0`: a static
binary that loads no C library. This ADR restates ADR-0015, and chooses the
runtime image.

## Decision drivers

- Keep the toolchain and images simple (the user's priority, ADR-0015).
- Keep the attack surface small, without making simplicity pay for it.
- The runtime image carries what nbpdns needs, and nothing it doesn't.

## Considered options

1. **For CI and builds:** glibc-based images (Debian or Ubuntu); Alpine
   and musl; both, chosen per image.
2. **For the runtime image:** distroless static; distroless base, with
   glibc; scratch.

## Decision outcome

**CI and builds: glibc-based images**, as ADR-0015 chose. The user decided
this on 2026-09-25:
> I have no problem switching to a Ubuntu base image, or a Debian base image,
> or similar. There's no constraints that require an Alpine image at this point
> in time. The same is true of the base image that we use for Docker builds.
> While I would prefer to keep the attack surface small, I would also prefer to
> make things simpler for the future.

- **CI** runs in the official `golang` image (Debian), pinned by digest.
- **Container builds** use Debian- or Ubuntu-based images, where they use
  any. ko builds nbpdns's image without one (ADR-0030).

**The runtime image: Debian's distroless static, non-root**, chosen by the
user on 2026-10-07:
- `gcr.io/distroless/static-debian13:nonroot`, or the debian12 variant if
  13 isn't published, pinned by its multi-arch index's digest.
- It holds CA certificates, time zones, `/etc/passwd` with the `nonroot`
  user (65532), and nothing else: no C library, no shell, no package
  manager.
- nbpdns needs no libc, so a glibc variant would only add a library to
  patch.

**The attack surface is kept small in other ways too:** image and
dependency scanning in CI, which comes with M17, and regular rebuilds.

### Consequences

- Good: one C library for every tool in CI, so published glibc binaries
  just work.
- Good: the runtime image is a few megabytes, with almost nothing to patch,
  and has no shell to run in it.
- Bad: no shell means no `docker exec` debugging inside the container.
  nbpdns's logs, `/status` and traces have to carry what's needed.
- Bad: glibc CI images are somewhat larger than Alpine's. That costs pulls,
  not the product.
- Bad: if nbpdns ever needs cgo, the runtime image must change to
  distroless base.

### Confirmation

- `make project-lint` checks that both forges' CI files use `CI_IMAGE` from
  the Makefile (ADR-0032).
- The release tests check that the binaries are static, and that the image
  runs as 65532, read-only, with no shell (ADR-0030).

## Pros and cons of the options

### Alpine and musl, for CI

- Good: smaller images.
- Bad: glibc release binaries, such as Vale's, don't run on it. Every tool
  would need a musl build.

### Distroless base, for the runtime

- Good: as ADR-0015 expected, and ready for cgo.
- Bad: glibc, which nbpdns doesn't load, makes the image larger and gives
  it more to patch.

### Scratch, for the runtime

- Good: the smallest image.
- Bad: nbpdns's build would have to carry the CA certificates, time zones
  and a passwd entry itself, and keep them current.

## More information

- Supersedes ADR-0015, whose decision about CI and builds this restates.
- ADR-0030 (the release), ADR-0032 (CI).
- Recorded in M05's design, 2026-10-07.
