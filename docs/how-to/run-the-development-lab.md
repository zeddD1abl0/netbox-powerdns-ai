---
title: Run the development lab
weight: 10
---

# Run the development lab

The development lab runs NetBox 4.7, with the NetBox DNS plugin, and a
PowerDNS 5.1 server, in containers. The integration tests run against it, and
the tutorials use it. This guide starts the lab, reaches NetBox and PowerDNS,
and removes the lab again.

## Before you start

You need:

- the repository, and its [development prerequisites](../../README.md#development),
  which include Docker;
- about 2.2 GB of free disk space and 1.5 GB of free memory.

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

3. Check that the PowerDNS 5.1 server answers with its version:

   ```shell
   curl -H "X-API-Key: nbpdns-lab-powerdns-api-key-not-for-production" \
     http://localhost:8151/api/v1/servers/localhost
   ```

The lab runs one NetBox for each
[supported release](../reference/supported-versions.md):

| NetBox | DNS plugin | URL |
|---|---|---|
| 4.7 | 1.7 | `http://localhost:8047` |

It has the superuser `admin`, with the password `nbpdns-lab-admin` and the
v2 API token `nbt_nbpdnslabadm.nbpdnsLabAdminTokenNotForProduction00000`.

The lab also runs one PowerDNS server for each
[supported PowerDNS release](../reference/supported-versions.md#powerdns). Each
is the primary of its own [server group](../reference/configuration.md#powerdnsgroups),
with no secondaries:

| Server group | PowerDNS | API URL |
|---|---|---|
| `lab-a` | 5.1 | `http://localhost:8151` |

It has the API key `nbpdns-lab-powerdns-api-key-not-for-production`, and keeps
its zones inside its container. Its API uses plain HTTP, so nbpdns warns that
the key crosses the network unencrypted.

> [!WARNING]
> These credentials are published in the repository, so the lab is for
> development only. On a local Docker host, its ports listen on `127.0.0.1`
> only. That includes a local Docker host reached over TCP, such as
> `tcp://localhost:2375` or `tcp://127.0.0.1:2375`.

If `DOCKER_HOST` names a remote Docker host, such as `tcp://docker:2375`, the
lab runs there. Its ports are then published on every interface of that
host, and the tests reach them through its name. `LAB_DOCKER_HOST`, if set,
takes the place of `DOCKER_HOST` for the lab and the tests only. GitHub's CI
job uses it, since a variable named `DOCKER_HOST` there would also redirect
the runner's own Docker commands.

## Run the integration tests

```shell
make test-integration
```

This starts the lab if it isn't running, then tests the packages that have
tests with the `integration` build tag: their unit tests too, but no other
package's, which `make test` covers. The tests use every NetBox and PowerDNS
server in the lab. Each test creates its own DNS data, and in NetBox users with their own API
tokens, with names that contain a random ID, and removes them when it ends.
Data you add to the lab yourself isn't touched.

The integration tests replay NetBox's webhooks, signed, rather than have
NetBox send them, which takes NetBox's worker.

## Have NetBox send webhooks

NetBox sends webhooks from a background worker, which the lab runs only
with the profile `webhooks`, since it takes about 300 MB more memory. Start
the lab with it:

```shell
make lab-up LAB_PROFILES=webhooks
```

The worker reaches services on the Docker host as `host.docker.internal`.
So a webhook in the lab's NetBox whose URL is
`http://host.docker.internal:8080/api/netbox-events` reaches `nbpdns serve`
listening on port 8080 of every interface. That works on a local Docker
host only.

To run the end-to-end webhook test, which starts the lab with the worker,
creates a webhook and an event rule in NetBox, changes a record, and waits
for `nbpdns serve` to see the zone drift:

```shell
make test-webhooks
```

It isn't part of `make ci`.
[Refresh drift as NetBox changes](refresh-drift-as-netbox-changes.md) sets
up webhooks outside the lab.

## Remove the lab

```shell
make lab-down
```

This removes the containers, the worker's too, and their data. The
downloaded images stay, so the next start is faster.
