# Objects the operator creates

For a `SereneDB` named `mydb`:

| Object | Name | Notes |
|---|---|---|
| StatefulSet | `mydb` | 1 replica, `pg_isready` probes, uid 999, all capabilities dropped, no service account token |
| ConfigMap | `mydb` | `serened.conf` flagfile, plus `pg_hba.conf` when `config.hba` is set. Its hash on the pod template restarts the pod on change |
| Secret | `mydb` | Generated password, only when `auth.existingSecret` is empty. Owned by the SereneDB only when `whenDeleted` is `Delete` |
| Service | `mydb` | Client entry point |
| Service | `mydb-hl` | Headless, governs the StatefulSet, publishes not-ready addresses |
| NetworkPolicy | `mydb` | Only when `networkPolicy.enabled` |
| PersistentVolumeClaim | `data-mydb-0` | Created by the StatefulSet from the claim template |

Every object carries `app.kubernetes.io/name=serenedb`,
`app.kubernetes.io/instance=<name>`,
`app.kubernetes.io/managed-by=serenedb-operator` and
`app.kubernetes.io/version=<tag>`. Selectors use the first two.

For a `Backup` named `nightly-20260924-020000`, a `VolumeSnapshot` of the same
name owned by the Backup.

## The pod

The container runs `serened /var/lib/serenedb` from the upstream image with the
ConfigMap mounted at `/etc/serenedb`; the image entrypoint appends
`--flagfile` itself. The password arrives as `POSTGRES_PASSWORD` from the
Secret. Probes run `pg_isready` against the pg-wire port: a startup probe with
60 attempts five seconds apart, then readiness every five seconds and liveness
every ten.

## What the operator patches

On the StatefulSet only `replicas`, the pod template, the update strategy and
the PVC retention policy, since the rest is immutable. On Services the type,
selector, ports and annotations, keeping the assigned ClusterIP and node ports.
Labels and annotations are merged, not replaced, so other controllers' keys
survive.
