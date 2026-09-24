# Introduction

The SereneDB Operator runs [SereneDB](https://github.com/serenedb/serenedb), the
Postgres-compatible search and analytics database, on Kubernetes. You declare a
`SereneDB` resource and the operator runs a single-node server with a persistent
data volume, generated superuser credentials, a flagfile ConfigMap, client and
headless Services, and reports readiness in the resource status. Five more kinds
manage what lives inside or around that server: databases, roles, the server's
own secrets manager, and volume snapshot backups on demand or on a schedule.

## One pod per server

SereneDB is a single-machine database, so a `SereneDB` always runs exactly one
pod. Image and configuration changes restart that pod. Expect a short downtime
window while the server checkpoints, restarts and reloads its indexes; clients
should reconnect on connection loss. There is no replication, failover or
connection pooling to configure, because the database has none to offer.

## The kinds

| Kind | What it manages |
|---|---|
| `SereneDB` | One server: image, storage, listeners, TLS, resources, scheduling |
| `Database` | A database inside a server, created with `CREATE DATABASE IF NOT EXISTS` |
| `DatabaseRole` | A role with attributes, a password from a Secret, and memberships |
| `ServerSecret` | A persistent entry in the server's secrets manager, for object storage and remote sources |
| `Backup` | One volume snapshot of the data volume, taken after `CHECKPOINT` |
| `ScheduledBackup` | Backups on a cron schedule with retention |

All kinds live in the API group `database.serenedb.com`, version `v1alpha1`.

## How it relates to the Helm chart

SereneDB ships a Helm chart in its repository, and the operator renders the same
objects the chart does, hardened the same way. Most chart values keep their names
as spec fields, so a chart deployment translates to a `SereneDB` almost line for
line. [Differences from the Helm chart](./reference/helm-differences.md) lists
the exceptions.

## Where to go next

[Installation](./installation.md) deploys the operator and its CRDs.
[Quickstart](./quickstart.md) creates a server, connects to it and adds a database
and a role. The guide then takes one topic per chapter, from
[running a server](./guide/server.md) through [backups](./guide/backups.md) to
[troubleshooting](./guide/troubleshooting.md), and the reference section has a
field table for every kind.
