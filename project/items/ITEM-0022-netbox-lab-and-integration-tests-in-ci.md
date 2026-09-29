---
id: ITEM-0022
title: NetBox lab and integration tests in CI
type: task # feature | bug | debt | task
status: done # open | in-progress | blocked | done | wontfix
milestone: M01
requirements: [REQ-036, REQ-039]
depends_on: []
created: 2026-09-27
closed: 2026-09-30
---

# ITEM-0022: NetBox lab and integration tests in CI

## Goal

A container lab with NetBox 4.7 and 4.6 and the DNS plugin. Integration
tests run against it locally and in every pipeline on both forges.

## Acceptance criteria

- [x] `deploy/dev/compose.yaml` runs PostgreSQL, Valkey, NetBox 4.7 with plugin 1.7.x (port 8047), and NetBox 4.6 with plugin 1.6.x (port 8046). Every image is pinned by digest, and the plugin by version.
- [x] docker-compose, and buildx if compose needs it, are pinned in `tools/tools.mk`. `tools/fetch.sh` can fetch an asset that is itself the binary.
- [x] `make lab-up` and `make lab-down` work. `make test-integration` depends on `lab-up`, and takes the lab host from `DOCKER_HOST`.
- [x] An `integration-test` job with a digest-pinned `docker:dind` service runs in every GitLab and GitHub pipeline. `make ci` includes `test-integration`, and `make project-lint` passes.
- [x] The job's peak ephemeral storage is measured and reported to the user.

## Notes

<!-- Append-only. Start each note with the date: "- YYYY-MM-DD: …" -->
- 2026-09-27: Created from M01's approved design, before implementation
  started.
- 2026-09-29: The user hasn't yet confirmed runner capacity for the
  Docker-in-Docker job (estimated 4–6 GB of ephemeral storage). Measure the
  real figure and report it. Also agreed in the review: golangci-lint and
  `go vet` must check files with the `integration` build tag, which they skip
  by default.
- 2026-09-30: Done.
  - **The lab** (`deploy/dev/compose.yaml`):
    - NetBox 4.7.1 (netbox-docker 5.1.1) with plugin 1.7.2 on port 8047;
    - NetBox 4.6.10 (netbox-docker 5.0.2) with plugin 1.6.1 on port 8046;
    - one PostgreSQL 18 server per NetBox, rather than one server with two
      databases, since a second database would need an init script;
    - one Valkey 9.1, with separate database numbers per NetBox.

    Every image is pinned by digest, using the Debian variants to match
    ADR-0015. Both NetBoxes have the superuser `admin` with the v2 token in
    `internal/lab`. v2 tokens need `API_TOKEN_PEPPER_1`, of at least 50
    characters.
  - **No image build (a change from the approved design).**
    - Each NetBox container runs `uv pip install netbox-plugin-dns==<pin>`,
      writes `plugins.py`, then runs netbox-docker's entrypoint, so there's
      no Dockerfile.
    - Compose v5 dropped its internal builder, so a build would also need
      buildx pinned and run through `buildx bake`. That variant was built
      and worked; the simpler one was kept.
    - Bind mounts are out anyway: under Docker-in-Docker they resolve on the
      Docker host.
  - **Found on the way:** netbox-docker 5.0.2 has no `/opt/netbox/health.sh`
    (it arrived in 5.1), so both NetBoxes use netbox-docker's `curl` check
    directly.
  - **Security:** the lab's credentials are public, so its ports bind to
    `127.0.0.1` on a local Docker host. For a `tcp://` `DOCKER_HOST`, as in
    CI, the Makefile binds them to `0.0.0.0` so the job can reach them by
    name. `.gitleaks.toml` allowlists only the lab's admin token, which the
    how-to quotes; any other token still fails the scan.
  - **Make and CI:**
    - `make lab-up` waits up to 60 s for the Docker daemon (Docker-in-Docker
      can still be starting), then `compose up --wait`. `make lab-down`
      removes the containers and volumes.
    - `make test-integration` depends on `lab-up`, and `make ci` includes
      it.
    - `go vet` and golangci-lint run with `-tags integration`.
    - Both forges have an `integration-test` job with a digest-pinned
      `docker:29.8.1-dind` service. On GitHub the build stage now waits for
      it.
  - **`internal/lab`:** the lab's facts for tests, with the host taken from
    `DOCKER_HOST`, and a smoke test checking each NetBox's version and
    plugin with the admin token.
  - **Emulated CI job**, run twice from cold: a local `docker:dind` service,
    and the pinned golang image as the job, with no Docker CLI and
    `DOCKER_HOST=tcp://docker:2375`.
    - `make test-integration` passed: 380 s for the build variant, 368 s for
      the final one. Most of that is NetBox's first-start migrations.
    - **Peak disk:** 3.3 GB in the Docker-in-Docker daemon (3.34 GB for the
      build variant), measured with `du -x`. Plain `du` gave about 6.2 GB,
      because it also walks the running containers' overlay mounts under
      `/var/lib/docker/rootfs`. Add 0.47 GB in the job container (Go modules
      119 MB, build cache 273 MB, tools 73 MB): about 3.8 GB in all.
    - The real pipelines haven't run yet. They first run on the next push.
  - **For the user:**
    - Runner capacity was never confirmed. Allow about 4 GB of ephemeral
      storage for the job.
    - The Docker-in-Docker storage should be on an emptyDir volume, since
      overlayfs can't run on the container's own overlay.
    - Each job pulls four images from Docker Hub anonymously, which its
      rate limits can refuse; a registry mirror or credentials on the
      runners avoid that.
  - `docs/how-to/run-the-development-lab.md` is written here, with the lab,
    rather than in ITEM-0024.
