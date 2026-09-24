# DatabaseRole

`databaseroles.database.serenedb.com`, namespaced. A role inside a server. Roles
are global to the server.

## Spec

| Field | Default | Description |
|---|---|---|
| `cluster.name` | required | The `SereneDB` in the same namespace |
| `name` | object name | Role name on the server. Immutable |
| `login` | `false` | May start client connections |
| `superuser` | `false` | Bypasses every permission check |
| `createDB` | `false` | May create databases |
| `createRole` | `false` | May create, alter and drop roles |
| `inherit` | `true` | Uses privileges of roles it is a member of |
| `connectionLimit` | unset | Stored by the server for compatibility, not enforced |
| `validUntil` | unset | Timestamp after which the password stops working |
| `passwordSecret.name` | unset | Kubernetes Secret holding the password |
| `passwordSecret.key` | `password` | Key inside that Secret |
| `inRoles` | `[]` | Complete list of memberships; others are revoked |
| `reclaimPolicy` | `Retain` | `Delete` drops the role with the object |

## Status

| Field | Meaning |
|---|---|
| `conditions[Ready]` | `Applied` when the role matches the spec. Other reasons: `ClusterNotFound`, `ClusterNotReady`, `ConnectionFailed`, `SQLError`, `PasswordSecretMissing`, `InvalidSpec` |
| `passwordSecretVersion` | `resourceVersion` of the Secret whose password was last applied |
| `observedGeneration` | Spec generation the status reflects |

## Behavior

Checks `pg_roles`, then runs `CREATE ROLE` or `ALTER ROLE` with every attribute.
The password is included on creation and whenever the Secret's `resourceVersion`
differs from `passwordSecretVersion`. Memberships are read from
`pg_auth_members` and converged with `GRANT` and `REVOKE`. Re-applied every ten
minutes and when the `SereneDB` or the password Secret changes.

Print columns: `CLUSTER`, `READY`, `AGE`.
