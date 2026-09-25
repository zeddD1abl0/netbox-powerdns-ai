---
title: "0015: glibc-based Debian or Ubuntu images for CI and containers"
status: accepted
date: 2026-09-25
decision-makers: [jordan]
requirements: [REQ-003, REQ-038]
questions: [Q-025]
supersedes:
---

# 0015: glibc-based Debian or Ubuntu images for CI and containers

## Context and problem statement

Container images can be based on glibc (Debian, Ubuntu, distroless Debian)
or on musl (Alpine). Alpine images are smaller, but third-party binaries built
against glibc, such as Vale's release binary, don't run on them. Supporting
both means building or fetching tools for more than one C library.

## Decision drivers

- Keep the toolchain and images simple (the user's priority).
- Keep the attack surface small, without making simplicity pay for it.

## Considered options

1. glibc-based images (Debian or Ubuntu) for CI and container builds.
2. Alpine/musl images, with tools built or fetched for musl.
3. Both, chosen per image.

## Decision outcome

Chosen option: **glibc-based images.** The user decided this on 2026-09-25:
> I have no problem switching to a Ubuntu base image, or a Debian base image,
> or similar. There's no constraints that require an Alpine image at this point
> in time. The same is true of the base image that we use for Docker builds.
> While I would prefer to keep the attack surface small, I would also prefer to
> make things simpler for the future.

- **CI** runs in the official `golang` image (Debian), pinned by digest.
- **Container builds** use Debian- or Ubuntu-based build images.
- **Attack surface** is kept small in other ways:
  - minimal glibc runtime images, such as distroless Debian or Ubuntu minimal;
  - image and dependency scanning in CI;
  - regular rebuilds.

  It isn't handled by switching to musl. The runtime image itself is chosen
  in M1 (Q-025).

### Consequences

- Good: one C library everywhere, so published glibc binaries just work.
- Bad: glibc base images are somewhat larger than Alpine. Scanning and
  minimal variants compensate for that.
- Q-025 (packaging) keeps its default of a distroless non-root image,
  narrowed to a glibc variant.

### Confirmation

- `make project-lint` checks that both forges' CI files use `CI_IMAGE` from
  the Makefile (ADR-0016).
- M1's image design is reviewed against this ADR.
