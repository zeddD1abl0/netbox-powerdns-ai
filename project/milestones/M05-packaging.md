---
id: M05
title: Packaging
status: planned # planned | in-progress | done
started:
closed:
---

# M05: Packaging

## Goal

nbpdns is released as static binaries and a container image, built the same way every time.

## Scope (provisional)

- GoReleaser, pinned like the other tools (ADR-0022)
- Static linux amd64 and arm64 binaries, with checksums
- The container image: a minimal, non-root runtime image, reviewed against ADR-0015 (Q-025)
- Where binaries and images are published, and whether they're signed, decided in this milestone's design

## Design, non-goals and acceptance criteria

To be written in this milestone's plan-mode session, before implementation
starts. The milestone's branch is `m05-packaging` (ADR-0010).
