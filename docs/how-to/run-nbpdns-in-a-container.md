---
title: Run nbpdns in a container
weight: 38
---

# Run nbpdns in a container

This guide runs `nbpdns serve` from its image: first with Docker, then in
Kubernetes. Each mounts the config file and the secrets read-only, and a
volume for the database, runs the container with a read-only root file
system, and checks its health.

## Before you start

You need:

- the image, as in [Install nbpdns](install-nbpdns.md#pull-the-image);
- a config file for the service, with NetBox's URL and token, as in
  [Configure nbpdns](configure-nbpdns.md). `serve` needs both, even with no
  server groups. The groups can be in the file, as in
  [Connect nbpdns to PowerDNS](connect-nbpdns-to-powerdns.md), or you can add
  them later with `nbpdns groups add`, as in
  [Manage server groups from the command line](manage-server-groups-from-the-command-line.md);
- NetBox's read-only token, and each primary's API key.

> [!WARNING]
> The service's port has no authentication until M10, and its status page
> and metrics name your server groups, zones, and URLs. Let only your
> probes and Prometheus reach it.

## What the image expects

- It runs `nbpdns` as user and group 65532, and has no shell. Give it a
  command, such as `serve`. Without one, it prints its help.
- `serve` keeps its state in a database, at `/var/lib/nbpdns/nbpdns.db` by
  default (`database.url`), and writes nothing else, so the root file
  system can be read-only. The image has no `/var/lib/nbpdns`: mount a
  volume there, writable by user 65532. `serve` creates the database, and
  applies its migrations, when it starts. Without the volume, it stops with:

  ```text
  the database's directory, /var/lib/nbpdns, doesn't exist; create it, or mount a volume there, writable by user 65532
  ```

  With its state in PostgreSQL, it needs no volume, as
  [Run nbpdns on PostgreSQL](run-nbpdns-on-postgresql.md) describes.
- Only one `serve` may use a database at a time. A second one refuses to
  start, and names the first one's host and PID.
- It logs JSON to standard error.
- It trusts the CA certificates in the image, Debian's, and any `ca_file`
  you mount.
- `serve` listens on `server.listen`, `:8080` by default, and stops cleanly
  on SIGTERM.

[Release artifacts](../reference/release-artifacts.md#container-image)
lists the image's settings.

## Run it with Docker

1. Write the config file, such as `/etc/nbpdns/nbpdns.yaml`. Name each
   secret's file by where the container sees it, in `/run/secrets`:

   ```yaml
   netbox:
     url: https://netbox.example.com
     token_file: /run/secrets/netbox-token
   powerdns:
     groups:
       - name: site-a
         views: [_default_]
         primary:
           url: https://pdns-a.example.com:8443
           api_key_file: /run/secrets/pdns-site-a
   ```

   The config file holds no secrets, so it can stay readable by anyone.

2. Put each secret in its own file, in a directory such as
   `/etc/nbpdns/secrets`: `netbox-token` and `pdns-site-a` here. Make them
   readable by user 65532 only:

   ```shell
   sudo chown -R 65532:65532 /etc/nbpdns/secrets
   sudo chmod 0700 /etc/nbpdns/secrets
   sudo chmod 0400 /etc/nbpdns/secrets/*
   ```

3. Make a directory for the database, owned by user 65532, and readable by
   nobody else:

   ```shell
   sudo install -d -m 0700 -o 65532 -g 65532 /var/lib/nbpdns
   ```

   > [!WARNING]
   > Don't mount a new Docker named volume, such as
   > `--volume nbpdns-data:/var/lib/nbpdns`, in its place. The image has no
   > such directory for Docker to copy an owner from, so the volume belongs
   > to root, and `serve` stops with an error that ends `permission denied;
   > make /var/lib/nbpdns writable by user 65532`. Use a bind mount of a
   > directory that user 65532 owns, as here.

   If you'll store secrets in the database, such as the API key of a group
   added with `nbpdns groups add`, or a secret setting, put the master key
   in the secrets directory too, and name it in the config file, with
   `database.master_key_file: /run/secrets/master-key`.
   [Protect stored secrets with a master key](protect-stored-secrets-with-a-master-key.md)
   makes one.

4. Start the container:

   ```shell
   docker run --detach --name nbpdns --restart unless-stopped \
     --read-only --cap-drop ALL --security-opt no-new-privileges \
     --publish 8080:8080 \
     --volume /etc/nbpdns/nbpdns.yaml:/etc/nbpdns/nbpdns.yaml:ro \
     --volume /etc/nbpdns/secrets:/run/secrets:ro \
     --volume /var/lib/nbpdns:/var/lib/nbpdns \
     --env NBPDNS_CONFIG=/etc/nbpdns/nbpdns.yaml \
     registry.example.com/group/netbox-powerdns-ai:0.1.0 serve
   ```

5. Wait for the first refresh, then check that it's ready:

   ```shell
   curl http://localhost:8080/readyz
   ```

   ```text
   ready
   ```

   Until the first refresh finishes, `/readyz` answers `503`. If it stays
   that way, read the logs, with `docker logs nbpdns`, and the status page,
   at `http://localhost:8080/status`.

`docker stop nbpdns` sends SIGTERM, and nbpdns exits with status 0.

## Run it in Kubernetes

1. Save the secrets, the config, the database's volume, and the
   deployment as `nbpdns.yaml`, with your own values:

   ```yaml
   apiVersion: v1
   kind: Secret
   metadata:
     name: nbpdns
   stringData:
     netbox-token: "<NetBox's read-only token>"
     pdns-site-a: "<site-a's PowerDNS API key>"
   ---
   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: nbpdns
   data:
     nbpdns.yaml: |
       netbox:
         url: https://netbox.example.com
         token_file: /run/secrets/nbpdns/netbox-token
       powerdns:
         groups:
           - name: site-a
             views: [_default_]
             primary:
               url: https://pdns-a.example.com:8443
               api_key_file: /run/secrets/nbpdns/pdns-site-a
   ---
   apiVersion: v1
   kind: PersistentVolumeClaim
   metadata:
     name: nbpdns-data
   spec:
     accessModes: [ReadWriteOnce]
     resources:
       requests:
         storage: 1Gi
   ---
   apiVersion: apps/v1
   kind: Deployment
   metadata:
     name: nbpdns
   spec:
     replicas: 1
     strategy:
       type: Recreate
     selector:
       matchLabels:
         app.kubernetes.io/name: nbpdns
     template:
       metadata:
         labels:
           app.kubernetes.io/name: nbpdns
       spec:
         automountServiceAccountToken: false
         securityContext:
           runAsNonRoot: true
           runAsUser: 65532
           runAsGroup: 65532
           fsGroup: 65532
           seccompProfile:
             type: RuntimeDefault
         containers:
           - name: nbpdns
             image: registry.example.com/group/netbox-powerdns-ai:0.1.0
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
               - name: data
                 mountPath: /var/lib/nbpdns
         volumes:
           - name: config
             configMap:
               name: nbpdns
           - name: secrets
             secret:
               secretName: nbpdns
               defaultMode: 0440
           - name: data
             persistentVolumeClaim:
               claimName: nbpdns-data
   ```

   - **One replica, recreated:** only one `serve` may use the database, so
     an update stops the old pod before it starts the new one. A rolling
     update would wait forever for a new pod that can't start while the old
     one holds the database. M09 adds instances that share the work.
   - **The database:** on the volume `nbpdns-data`, which `fsGroup` makes
     writable by group 65532. Use storage on a local disk, not NFS, which
     SQLite's locks don't trust.
   - **Readiness:** `/readyz` is ready once nbpdns has restored its last
     reports from the database, or, on its first start, once its first
     refresh has finished.
   - **Liveness:** `/livez` fails only when no refresh has started for
     `drift.interval` + `drift.timeout` + 1 minute, 16 minutes by default.
     Neither probe depends on NetBox or the primaries, so their outages
     don't restart the pod.
   - **Secrets:** the files are readable by group 65532, `fsGroup`, and
     nbpdns reads them through the `_file` keys.

2. Apply it, and wait for the pod to be ready:

   ```shell
   kubectl apply -f nbpdns.yaml
   kubectl rollout status deployment/nbpdns
   ```

3. Read the status page through a port forward:

   ```shell
   kubectl port-forward deployment/nbpdns 8080:8080
   curl http://localhost:8080/status
   ```

## Next steps

- [How nbpdns stores its state](../explanation/how-nbpdns-stores-its-state.md)
  explains the database on the volume, and
  [Back up nbpdns's database](back-up-nbpdns-database.md) copies it.
- [Monitor drift with Prometheus](monitor-drift-with-prometheus.md).
- [Service endpoints](../reference/service-endpoints.md) describes what
  each endpoint answers.
