# Database

`databases.database.serenedb.com`, namespaced. A database inside a server.

## Spec

| Field | Default | Description |
|---|---|---|
| `cluster.name` | required | The `SereneDB` in the same namespace |
| `name` | object name | Database name on the server. Immutable |
| `reclaimPolicy` | `Retain` | `Retain` keeps the database when the object is deleted, `Delete` drops it |

## Status

| Field | Meaning |
|---|---|
| `conditions[Ready]` | `Applied` when the database exists. Other reasons: `ClusterNotFound`, `ClusterNotReady`, `ConnectionFailed`, `SQLError`, `InvalidSpec` |
| `observedGeneration` | Spec generation the status reflects |

## Behavior

Checks `pg_database`, runs `CREATE DATABASE IF NOT EXISTS` when missing. Carries
the finalizer `database.serenedb.com/cleanup`; with `Delete` the finalizer runs
`DROP DATABASE`, and with the `SereneDB` gone it is removed without SQL.
Re-applied every ten minutes and when the `SereneDB` changes.

Print columns: `CLUSTER`, `READY`, `AGE`.
