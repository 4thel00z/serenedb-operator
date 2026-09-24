# Passwords

The operator passes the superuser password to the server through the
`POSTGRES_PASSWORD` environment variable. SereneDB honors that variable only on
the first boot with an empty data directory. After that, the password lives in
the data directory and changes only through SQL:

```sql
ALTER ROLE postgres PASSWORD '...';
```

Changing the Secret later has no effect on a server that has already been
initialized.

## The generated Secret

When `auth.existingSecret` is empty, the operator generates a 24-character
password, stores it in a Secret named after the `SereneDB` under the key
`postgres-password`, and never rewrites it.

The Secret's lifetime follows the data volume, not the `SereneDB`. With the
default `persistence.retentionPolicy.whenDeleted: Retain`, deleting the
`SereneDB` leaves the PVC and the Secret behind together. Recreating the
`SereneDB` adopts both, so the password the data directory was initialized with
is still the one in the Secret. With `whenDeleted: Delete`, the Secret carries
an owner reference and is garbage collected with the rest.

## Bringing your own

```yaml
spec:
  auth:
    existingSecret: mydb-superuser
    passwordKey: postgres-password
```

Create the Secret before the `SereneDB`. Until the Secret and key exist, the
`Ready` condition reports `SecretMissing` or `SecretKeyMissing` and the
StatefulSet is not created, so the server never boots with the wrong password.

## Role passwords

`DatabaseRole` passwords also come from Secrets, but those are re-applied: the
operator records the Secret's `resourceVersion` in the role's status and issues
`ALTER ROLE ... PASSWORD` whenever it changes. See
[Databases and roles](./databases-roles.md).
