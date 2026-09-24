<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo-lockup-dark.svg">
    <img alt="SereneDB Operator" src="assets/logo-lockup.svg" width="560">
  </picture>
</p>

<p align="center">
  <strong>Run SereneDB on Kubernetes. One resource per server, declarative databases, roles and storage credentials, volume snapshot backups.</strong>
</p>

<p align="center">
  <a href="https://github.com/4thel00z/serenedb-operator/actions/workflows/test.yml"><img src="https://github.com/4thel00z/serenedb-operator/actions/workflows/test.yml/badge.svg" alt="tests"></a>
  <a href="https://github.com/4thel00z/serenedb-operator/actions/workflows/test-e2e.yml"><img src="https://github.com/4thel00z/serenedb-operator/actions/workflows/test-e2e.yml/badge.svg" alt="e2e"></a>
  <a href="https://4thel00z.github.io/serenedb-operator/"><img src="https://img.shields.io/badge/docs-book-blue?logo=mdbook&logoColor=white" alt="Documentation"></a>
  <img src="https://img.shields.io/badge/kubernetes-1.27%2B-326ce5?logo=kubernetes&logoColor=white" alt="Kubernetes 1.27+">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-blue.svg" alt="Apache-2.0"></a>
</p>

<p align="center">
  <a href="https://4thel00z.github.io/serenedb-operator/">Docs</a> ·
  <a href="https://4thel00z.github.io/serenedb-operator/quickstart.html">Quickstart</a> ·
  <a href="https://github.com/serenedb/serenedb">SereneDB</a>
</p>

---

[SereneDB](https://github.com/serenedb/serenedb) is a Postgres-compatible search and
analytics database that runs as a single node. This operator gives it a home on
Kubernetes: a `SereneDB` resource becomes a hardened StatefulSet with a persistent
volume, generated credentials and Services, and five more kinds manage what lives
inside that server without anyone opening a `psql` session.

## Highlights

- **One resource, one server.** Image, storage, listeners, TLS, resources and
  scheduling in a single spec that mirrors the upstream Helm chart's values.
- **Status you can wait on.** `Ready` and `Progressing` conditions, the rolled-out
  version, and connection details in `.status`.
- **Databases and roles as objects.** `Database` and `DatabaseRole` run idempotent,
  quoted SQL over pg-wire, converge attributes and memberships, and rotate
  passwords when a Secret changes.
- **Storage credentials without SQL history.** `ServerSecret` syncs a Kubernetes
  Secret into the server's secrets manager for S3, GCS, Azure, Iceberg and more.
- **Snapshot backups.** `Backup` runs `CHECKPOINT` and takes a CSI volume snapshot;
  `ScheduledBackup` does it on a cron schedule with retention; `bootstrap` restores
  a new server from one.
- **Nothing to lock you out.** The generated password Secret lives as long as the
  data volume, and immutable storage fields are rejected by the API server instead
  of failing at rollout.

## Install

```sh
make install                                              # CRDs
make deploy IMG=ghcr.io/4thel00z/serenedb-operator:latest # operator
```

Requires Kubernetes 1.27 or newer. Backups need a CSI driver with snapshot support.

## A server in twelve lines

```yaml
apiVersion: database.serenedb.com/v1alpha1
kind: SereneDB
metadata:
  name: mydb
spec:
  image: {tag: "26.09.2"}
  persistence: {size: 50Gi}
  resources:
    requests: {cpu: "2", memory: 4Gi}
    limits: {cpu: "2", memory: 4Gi}
  config: {cpuThreads: 2, ioThreads: 2}
```

```sh
kubectl apply -f mydb.yaml
kubectl get sdb mydb
```

```
NAME   READY   VERSION   AGE
mydb   True    26.09.2   40s
```

```sh
PGPASSWORD=$(kubectl get secret mydb -o jsonpath='{.data.postgres-password}' | base64 -d)
psql -h mydb.<namespace>.svc -p 7890 -U postgres
```

## Kinds

| Kind | Manages |
|---|---|
| `SereneDB` | One server with its volume, Services, config and credentials |
| `Database` | A database, `CREATE DATABASE IF NOT EXISTS`, optional drop on delete |
| `DatabaseRole` | A role: attributes, password from a Secret, memberships |
| `ServerSecret` | A persistent entry in the server's secrets manager |
| `Backup` | One volume snapshot after `CHECKPOINT` |
| `ScheduledBackup` | Backups on a cron schedule, with `keep` |

The [documentation](https://4thel00z.github.io/serenedb-operator/) has a guide per
topic and a field reference per kind.

## Development

```sh
make test       # unit and envtest
make lint
make test-e2e   # kind cluster against a real SereneDB
mdbook serve docs --open
```

## License

Apache 2.0, the same as SereneDB.
