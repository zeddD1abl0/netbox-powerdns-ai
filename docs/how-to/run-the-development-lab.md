---
title: Run the development lab
weight: 10
---

# Run the development lab

The development lab runs NetBox 4.7 and NetBox 4.6, each with the NetBox DNS
plugin, in containers. The integration tests run against it, and the
tutorials use it. This guide starts the lab, reaches each NetBox, and removes
the lab again.

## Before you start

You need:

- the repository, and its [development prerequisites](../../README.md#development),
  which include Docker;
- about 4 GB of free disk space and 2 GB of free memory.

The lab's own tool, docker-compose, is pinned in the repository and fetched on
first use.

## Start the lab

1. Start it:

   ```shell
   make lab-up
   ```

   The first start downloads the images and applies every NetBox database
   migration, which takes several minutes. Later starts take seconds. The
   command returns once every container reports healthy.

2. Check that NetBox 4.7 answers with its version and the plugin's:

   ```shell
   curl -H "Authorization: Bearer nbt_nbpdnslabadm.nbpdnsLabAdminTokenNotForProduction00000" \
     http://localhost:8047/api/status/
   ```

The lab's NetBox instances:

| NetBox | DNS plugin | URL |
|---|---|---|
| 4.7 | 1.7 | `http://localhost:8047` |
| 4.6 | 1.6 | `http://localhost:8046` |

Each has the superuser `admin`, with the password `nbpdns-lab-admin` and the
v2 API token `nbt_nbpdnslabadm.nbpdnsLabAdminTokenNotForProduction00000`.

> [!WARNING]
> These credentials are published in the repository, so the lab is for
> development only. On a local Docker host, its ports listen on `127.0.0.1`
> only.

If `DOCKER_HOST` names a remote Docker host, such as `tcp://docker:2375`, the
lab runs there. Its ports are then published on every interface of that
host, and the tests reach them through its name.

## Run the integration tests

```shell
make test-integration
```

This starts the lab if it isn't running, then runs every test with the
`integration` build tag, against both NetBox versions.

## Remove the lab

```shell
make lab-down
```

This removes the containers and their data. The downloaded images stay, so
the next start is faster.
