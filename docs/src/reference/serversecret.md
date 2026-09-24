# ServerSecret

`serversecrets.database.serenedb.com`, namespaced. A persistent entry in the
server's secrets manager.

## Spec

| Field | Default | Description |
|---|---|---|
| `cluster.name` | required | The `SereneDB` in the same namespace |
| `name` | object name | Secret name on the server. Immutable |
| `type` | required | `azure`, `gcs`, `http`, `huggingface`, `iceberg`, `postgres`, `r2` or `s3` |
| `scope` | unset | One path prefix the secret applies to |
| `options` | `{}` | Non-sensitive `CREATE SECRET` options by name |
| `valuesFrom.name` | unset | Kubernetes Secret whose every key becomes an option; wins over `options` |

## Status

| Field | Meaning |
|---|---|
| `conditions[Ready]` | `Applied` when the statement ran. Other reasons: `ClusterNotFound`, `ClusterNotReady`, `ConnectionFailed`, `SQLError`, `ValuesSecretMissing`, `InvalidSpec` |
| `valuesSecretVersion` | `resourceVersion` of the Kubernetes Secret last applied |
| `observedGeneration` | Spec generation the status reflects |

## Behavior

Runs `CREATE OR REPLACE PERSISTENT SECRET` on every pass, options in sorted
order. Option names must match `^[A-Za-z_][A-Za-z0-9_]*$`. Always runs
`DROP SECRET` on deletion when the entry exists. The server stores persistent
secrets unencrypted on the data volume.

Print columns: `CLUSTER`, `TYPE`, `READY`, `AGE`.
