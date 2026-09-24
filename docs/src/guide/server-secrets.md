# Server secrets

SereneDB reads Parquet, Iceberg, CSV and JSON straight from object storage, HTTP
endpoints, Hugging Face and remote databases. The credentials for those sources
live in the server's secrets manager, created with `CREATE SECRET`. A
`ServerSecret` puts an entry there from a Kubernetes Secret, so cloud
credentials never appear in SQL history and can rotate with the Secret.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: lake-s3-credentials
stringData:
  KEY_ID: AKIA...
  SECRET: ...
---
apiVersion: database.serenedb.com/v1alpha1
kind: ServerSecret
metadata:
  name: lake
spec:
  cluster: {name: mydb}
  type: s3
  scope: s3://my-lake/
  options: {REGION: eu-central-1}
  valuesFrom: {name: lake-s3-credentials}
```

The operator renders

```sql
CREATE OR REPLACE PERSISTENT SECRET "lake" (TYPE s3, SCOPE 's3://my-lake/', KEY_ID '...', REGION 'eu-central-1', SECRET '...')
```

and runs it on every pass. `options` holds the non-sensitive settings inline;
every key of the `valuesFrom` Secret becomes an option too, and wins on
conflict. Option names must be plain identifiers such as `KEY_ID`, `SECRET`,
`REGION` or `ENDPOINT`; the accepted set per type is in the SereneDB
documentation on the secrets manager.

## Types and scope

`type` is one of `azure`, `gcs`, `http`, `huggingface`, `iceberg`, `postgres`,
`r2` and `s3`. `scope` is one path prefix the secret applies to; when several
secrets of a type match a path, the server picks the longest prefix. Leave
`scope` empty to apply to every path of the type, or create one `ServerSecret`
per prefix.

## What to know

- Persistent server secrets are stored unencrypted on the data volume. A
  `ServerSecret` copies credentials from a Kubernetes Secret into that file.
  Treat the data volume accordingly.
- The published SereneDB binary offers only the `config` provider, which takes
  static keys. There is no `credential_chain` provider, so IRSA and workload
  identity do not apply; supply keys through `valuesFrom`.
- A `ServerSecret` is always dropped from the server when the object is
  deleted. Credentials are not data worth retaining.
- Server secrets survive server restarts and upgrades, since they are
  persistent. They are part of the data volume, so a
  [volume snapshot backup](./backups.md) carries them along.
