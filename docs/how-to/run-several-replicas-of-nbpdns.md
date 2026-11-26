---
title: Run several replicas of nbpdns
weight: 38
---

# Run several replicas of nbpdns

This guide runs several `nbpdns serve`s on one PostgreSQL database, as
replicas, so that nbpdns keeps answering when one of them stops, dies or
loses its network. One replica, the leader, refreshes the drift report and
prunes the database. Every replica serves the same API answers and status,
from the database, so a load balancer can send any request to any of them.
When the leader goes, another takes over: within seconds if it stops or
dies, and within about 30 seconds if its host or network fails.

[How nbpdns replicas choose a leader](../explanation/how-nbpdns-replicas-choose-a-leader.md)
explains how they choose it, and why two never lead at once.

## Before you start

You need:

- a PostgreSQL 18 database for nbpdns, and its `database.url`, as
  [Run nbpdns on PostgreSQL](run-nbpdns-on-postgresql.md) sets them up.
  SQLite runs one `serve` only;
- somewhere to run two or more replicas, preferably on different hosts:
  Kubernetes, or Docker Compose, as below;
- a load balancer in front of them, such as a Kubernetes Service, for the
  API, the status page, and NetBox's webhooks.

## Give every replica the same configuration

Each replica reads its own copy of the configuration, so give them all the
same:

- **The same `database.url`.** Replicas that name different databases
  don't share anything: each leads its own.
- **The same config file.** Each replica copies the file's server groups
  into the database as it starts, so the last one to start sets them: one
  that starts with an older file puts its groups back. Change the file for
  every replica, then restart them.
- **The same secrets**, the NetBox token and webhook secret, and the same
  `database.master_key`, if you store secrets in the database.
- **`database.listen: true`**, unless something between nbpdns and
  PostgreSQL drops its notifications. Each replica then hears of every
  change at once, rather than at its next check of the database, every
  `database.poll_interval`, 5 seconds by default. It costs each replica one
  more connection. Each replica logs `listening for the database's changes`
  once it listens, and a warning if that connection is lost, then listens
  again; its checks go on meanwhile.

The command line works beside the replicas: `nbpdns settings` and
`nbpdns groups` change the database while they run, and each change
reaches every replica.
`nbpdns db migrate` and `nbpdns secrets rotate-master-key` refuse while any
replica runs, and a replica refuses to start while either runs. Each
refusal names the replicas by their host and PID:

```text
2 other nbpdns processes hold the database's lock: host nbpdns-0, PID 1, since 2026-10-22T09:15:27Z; host nbpdns-1, PID 1, since 2026-10-22T09:15:31Z; stop them first, or give this one another database.url
```

PostgreSQL shows each replica's sessions in `pg_stat_activity`, named so in
`application_name`. It keeps 63 bytes of it, so a host name too long to fit
beside the PID is cut short in its middle, at `...`.

## Run them in Kubernetes

This deployment runs three replicas, spread across nodes, behind a Service.
It needs no volume: the state is in PostgreSQL.

1. Save the secrets, the config, the deployment and the Service as
   `nbpdns.yaml`, with your own values:

   ```yaml
   apiVersion: v1
   kind: Secret
   metadata:
     name: nbpdns
   stringData:
     database-url: "postgres://nbpdns:<password>@db.example.com/nbpdns?sslmode=verify-full&sslrootcert=system"
     netbox-token: "<NetBox's read-only token>"
     netbox-webhook-secret: "<the webhook's secret>"
     pdns-site-a: "<site-a's PowerDNS API key>"
   ---
   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: nbpdns
   data:
     nbpdns.yaml: |
       database:
         url_file: /run/secrets/nbpdns/database-url
         listen: true
       netbox:
         url: https://netbox.example.com
         token_file: /run/secrets/nbpdns/netbox-token
         webhook_secret_file: /run/secrets/nbpdns/netbox-webhook-secret
       powerdns:
         groups:
           - name: site-a
             views: [_default_]
             primary:
               url: https://pdns-a.example.com:8443
               api_key_file: /run/secrets/nbpdns/pdns-site-a
   ---
   apiVersion: apps/v1
   kind: Deployment
   metadata:
     name: nbpdns
   spec:
     replicas: 3
     strategy:
       type: RollingUpdate
       rollingUpdate:
         maxUnavailable: 1
     selector:
       matchLabels:
         app.kubernetes.io/name: nbpdns
     template:
       metadata:
         labels:
           app.kubernetes.io/name: nbpdns
       spec:
         automountServiceAccountToken: false
         topologySpreadConstraints:
           - maxSkew: 1
             topologyKey: kubernetes.io/hostname
             whenUnsatisfiable: ScheduleAnyway
             labelSelector:
               matchLabels:
                 app.kubernetes.io/name: nbpdns
         securityContext:
           runAsNonRoot: true
           runAsUser: 65532
           runAsGroup: 65532
           fsGroup: 65532
           seccompProfile:
             type: RuntimeDefault
         containers:
           - name: nbpdns
             image: registry.example.com/group/netbox-powerdns-ai:0.9.0
             args: [serve]
             env:
               - name: NBPDNS_CONFIG
                 value: /etc/nbpdns/nbpdns.yaml
             ports:
               - name: http
                 containerPort: 8080
             readinessProbe:
               httpGet:
                 path: /readyz
                 port: http
               periodSeconds: 10
             livenessProbe:
               httpGet:
                 path: /livez
                 port: http
               periodSeconds: 30
               failureThreshold: 3
             securityContext:
               readOnlyRootFilesystem: true
               allowPrivilegeEscalation: false
               capabilities:
                 drop: [ALL]
             volumeMounts:
               - name: config
                 mountPath: /etc/nbpdns
                 readOnly: true
               - name: secrets
                 mountPath: /run/secrets/nbpdns
                 readOnly: true
         volumes:
           - name: config
             configMap:
               name: nbpdns
           - name: secrets
             secret:
               secretName: nbpdns
               defaultMode: 0440
   ---
   apiVersion: v1
   kind: Service
   metadata:
     name: nbpdns
   spec:
     selector:
       app.kubernetes.io/name: nbpdns
     ports:
       - name: http
         port: 8080
         targetPort: http
   ```

   - **A rolling update:** replicas share the database, so a new pod starts
     beside the old ones. The first of a new release to start applies its
     migrations. A replica of an older release then refuses to start on
     the newer schema, so roll forward, not back.
   - **Spread across nodes:** `topologySpreadConstraints` keeps the
     replicas apart, so one node's loss takes one replica.
   - **Readiness:** a replica is ready once it has loaded a refresh from the
     database: at once, unless the database has none yet, as on its first
     start, when the replicas wait for the leader's first refresh.
   - **The database's URL** is a secret, read through `database.url_file`,
     so its password stays out of the ConfigMap and the pod's environment.

2. Apply it, and wait for the replicas to be ready:

   ```shell
   kubectl apply -f nbpdns.yaml
   kubectl rollout status deployment/nbpdns
   ```

3. Point NetBox's webhook, and the API's clients, at the Service, such as
   `http://nbpdns.<namespace>.svc:8080/api/netbox-events`. Any replica takes
   a webhook: the one that takes it queues it in the database, and the
   leader refreshes its zones.
   [Refresh drift as NetBox changes](refresh-drift-as-netbox-changes.md)
   sets the webhook up.

## Run them with Docker Compose

This runs three replicas on one host, each on a port of its own. It guards
against a replica that stops or dies, but not against the host's loss.

1. Write the config file and the secrets as
   [Run nbpdns in a container](run-nbpdns-in-a-container.md#run-it-with-docker)
   does, with no directory for the database, and with the database in the
   config file:

   ```yaml
   database:
     url_file: /run/secrets/database-url
     listen: true
   ```

   Put the URL, password and all, in `/etc/nbpdns/secrets/database-url`,
   readable by user 65532 only, as the other secrets are.

2. Save the replicas as `compose.yaml`:

   ```yaml
   x-nbpdns: &nbpdns
     image: registry.example.com/group/netbox-powerdns-ai:0.9.0
     command: [serve]
     restart: unless-stopped
     read_only: true
     cap_drop: [ALL]
     security_opt: [no-new-privileges]
     environment:
       NBPDNS_CONFIG: /etc/nbpdns/nbpdns.yaml
     volumes:
       - /etc/nbpdns/nbpdns.yaml:/etc/nbpdns/nbpdns.yaml:ro
       - /etc/nbpdns/secrets:/run/secrets:ro

   services:
     nbpdns-1:
       <<: *nbpdns
       hostname: nbpdns-1
       ports: ["8081:8080"]
     nbpdns-2:
       <<: *nbpdns
       hostname: nbpdns-2
       ports: ["8082:8080"]
     nbpdns-3:
       <<: *nbpdns
       hostname: nbpdns-3
       ports: ["8083:8080"]
   ```

   Each replica has a host name of its own, which names it on `/status`,
   in the audit trail, and in a lock holder's error.

3. Start them, and check that each is ready:

   ```shell
   docker compose up --detach
   curl http://localhost:8081/readyz http://localhost:8082/readyz http://localhost:8083/readyz
   ```

4. Put a load balancer in front of the three ports, and point NetBox's
   webhook and the API's clients at it.

## Check which replica leads

Every replica's status page names itself, its role, and the leader:

```shell
curl http://localhost:8082/status
```

```text
Replica: nbpdns-2, ID 4GYF4HLO4YFZYBT6VWRXLAJVNX, a follower of nbpdns-1, ID TFJWTP3YERUNROVXXE64VEW4J7, the leader since 2026-11-25T06:24:07Z, last seen 2026-11-25T06:24:22Z
```

`/status?json=1` and `/api/status` have the same, in `replica` and
`leader`:

```json
"replica": {
  "name": "nbpdns-2",
  "id": "4GYF4HLO4YFZYBT6VWRXLAJVNX",
  "role": "follower"
},
"leader": {
  "name": "nbpdns-1",
  "id": "TFJWTP3YERUNROVXXE64VEW4J7",
  "since": "2026-11-25T06:24:07.935149Z",
  "last_seen": "2026-11-25T06:24:32.940934Z",
  "next_refresh": "2026-11-25T06:29:07.94177Z"
}
```

Every replica's status has the same deployment's facts, such as the last
refresh, the server groups and the webhooks' queue, and facts of its own,
such as the refreshes it ran; the
[service endpoints reference](../reference/service-endpoints.md#status)
says which are which.

The ID is random, from the replica's start, so it tells a restarted
replica from the one before it, under the same name. Each takeover is a
`service.leader_acquired` event in the audit trail, with the leader before
it and when that leader was last seen:

```shell
nbpdns audit list --action service.leader_acquired
```

```text
SEQ  TIME                  ACTOR   ACTION                   TARGET            OUTCOME  REASON
7    2026-11-25T06:24:07Z  system  service.leader_acquired  service:nbpdns-1  success  -
16   2026-11-25T06:25:48Z  system  service.leader_acquired  service:nbpdns-2  success  -
```

## Monitor them

Scrape each replica, as a target of its own.
[Monitor drift with Prometheus](monitor-drift-with-prometheus.md#monitor-several-replicas)
aggregates the drift alerts, so that each fires once. `nbpdns_leader` is 1
on the leader and 0 on the others, so add a rule that fires when none
leads, or the replicas disagree:

```yaml
- alert: NbpdnsNoLeader
  expr: sum(nbpdns_leader) != 1
  for: 2m
  annotations:
    summary: "{{ $value }} nbpdns replicas lead, rather than one"
```

A takeover can take about 30 seconds, so `for: 2m` lets one finish.
`nbpdns_leader_changes_total` counts the takeovers that each replica has
seen, and `nbpdns_leader_since_timestamp_seconds` says when the leader took
over.

## Stop and replace replicas

- **Stop a replica** with SIGTERM, as `kubectl delete pod` and
  `docker compose stop` do. A leader that stops releases its lock, and a
  follower takes over at its next check of the database, or at once with
  `database.listen`.
- **A replica that dies,** killed or crashed, takes its lock with it, and a
  follower takes over at its next check.
- **A replica whose host dies, whose network fails, or that hangs** loses
  its lock within about 30 seconds, as PostgreSQL ends its session. A
  follower then takes over, and refreshes at once. A leader that loses its
  network steps down first, within 10 seconds, and one that hangs steps
  down as soon as it resumes, without writing.
- **Migrate or rotate the master key** with every replica stopped: scale
  the deployment to 0, or `docker compose stop`, then run
  `nbpdns db migrate` or `nbpdns secrets rotate-master-key`, then start them
  again.

## Next steps

- [How nbpdns replicas choose a leader](../explanation/how-nbpdns-replicas-choose-a-leader.md)
  explains the timings, and what keeps a lost leader from writing.
- [Back up nbpdns's database](back-up-nbpdns-database.md#back-up-a-postgresql-database)
  copies it with `pg_dump`, while the replicas run.
- The [configuration reference](../reference/configuration.md#databaselisten)
  describes `database.listen` and `database.poll_interval`.
