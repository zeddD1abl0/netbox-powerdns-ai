---
title: Give nbpdns read-only access to NetBox
weight: 20
---

# Give nbpdns read-only access to NetBox

nbpdns reads DNS data from NetBox with an API token. Give it a token whose
user can only view the DNS plugin's objects, so that the token can't change
NetBox or read anything else in it. This guide creates that user, its
permission, and its token, then checks them with nbpdns.

## Before you start

You need:

- NetBox 4.7, with the NetBox DNS plugin (see
  [Supported versions](../reference/supported-versions.md));
- a NetBox account that can manage users, permissions, and API tokens;
- nbpdns, configured with NetBox's URL (see [Configure nbpdns](configure-nbpdns.md)).

## Create the user, permission, and token

1. In NetBox's administration menu, under **Authentication**, open
   **Users**, and add a user named `nbpdns`. Give it a long random password; nobody signs in as it. Leave it
   without staff or superuser status.
2. Under **Authentication**, open **Permissions**, and add a permission:
   - **Name:** `nbpdns-read-dns`;
   - **Enabled:** on;
   - **Actions:** only **Can view**;
   - **Object types:** the NetBox DNS plugin's **View**, **Zone**,
     **Nameserver**, and **Record**;
   - **Users:** `nbpdns`.
3. Under **Authentication**, open **API Tokens**, and add a token for the
   user `nbpdns`. Keep the default, a v2 token, and turn **Write enabled**
   off. If you can, set an expiry date, and allow only the addresses nbpdns
   connects from.
4. Copy the token. It starts with `nbt_`, and NetBox shows it only once.

## Or create them with the REST API

If you manage NetBox with scripts, make the same objects with three requests.
`ADMIN_TOKEN` is a token that can manage users.

1. Create the user, and note the `id` in NetBox's answer:

   ```shell
   curl -X POST "$NETBOX/api/users/users/" \
     -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
     -d '{"username": "nbpdns", "password": "<a long random password>"}'
   ```

2. Create the permission, with the user's `id` in `users`:

   ```shell
   curl -X POST "$NETBOX/api/users/permissions/" \
     -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
     -d '{"name": "nbpdns-read-dns", "actions": ["view"],
          "object_types": ["netbox_dns.view", "netbox_dns.zone",
                           "netbox_dns.nameserver", "netbox_dns.record"],
          "users": [<id>]}'
   ```

3. Create the token:

   ```shell
   curl -X POST "$NETBOX/api/users/tokens/" \
     -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
     -d '{"user": {"username": "nbpdns"}, "description": "nbpdns", "write_enabled": false}'
   ```

   The answer holds the token in two parts, `key` and `token`. The token
   that nbpdns uses is `nbt_<key>.<token>`. NetBox shows the `token` part
   only once.

## Give the token to nbpdns

1. Save the token in a file that only nbpdns's operating system user can
   read, such as `/etc/nbpdns/netbox-token`, with mode `600`. A final newline
   is ignored.
2. Point nbpdns at the file in its config file:

   ```yaml
   netbox:
     url: https://netbox.example.com
     token_file: /etc/nbpdns/netbox-token
   ```

   Or set `NBPDNS_NETBOX_TOKEN_FILE=/etc/nbpdns/netbox-token` in its
   environment.

> [!WARNING]
> Don't pass the token with `--netbox-token`: other users of the host can see
> a command's flags in the process list.

## Check the access

Run:

```shell
nbpdns netbox check
```

Every row should be `ok`:

```text
CHECK                  RESULT   DETAIL
connection             ok       https://
netbox                 ok       NetBox 4.7.1
plugin                 ok       netbox_dns 1.7.2
token                  ok       a v2 token, accepted
netbox_dns.view        ok       can view 2
netbox_dns.zone        ok       can view 40
netbox_dns.nameserver  ok       can view 3
netbox_dns.record      ok       can view 1250
```

If a row isn't, its detail says why:

| Row | Result | Fix |
|---|---|---|
| `token` | `failed`: NetBox rejected the token | Check that the file holds the whole token, starting with `nbt_`, and that the token is enabled and hasn't expired. |
| `netbox_dns.…` | `failed`: the token's user can't view these objects | Add that object type to the permission, and check that the permission is enabled and assigned to the user. |
| `token` | `warning`: a v1 token | Create a v2 token instead. |
| `connection` | `warning`: `http://` | Use NetBox's `https://` URL, if it has one, so the token isn't sent unencrypted. |

## Related

- [Configuration reference](../reference/configuration.md): `netbox.token`,
  `netbox.url`, and `netbox.ca_file`.
- [How nbpdns reads NetBox](../explanation/how-nbpdns-reads-netbox.md).
