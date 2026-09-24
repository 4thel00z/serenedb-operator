# Backups and restore

`Backup` and `ScheduledBackup` snapshot the data volume through the CSI snapshot
API. The operator runs `CHECKPOINT` on the server, creates a `VolumeSnapshot` of
the data PVC, and reports `Completed` once the snapshot is ready to use.

## Requirements

The cluster needs a CSI driver with snapshot support, a `VolumeSnapshotClass`,
and the external snapshot controller that serves the
`snapshot.storage.k8s.io/v1` API. Without that API a `Backup` reports `Failed`
with a message saying so, and every other kind keeps working. The operator
checks for the API at startup and only then watches snapshots; otherwise it
polls a running backup every fifteen seconds.

## A one-off backup

```yaml
apiVersion: database.serenedb.com/v1alpha1
kind: Backup
metadata:
  name: before-upgrade
spec:
  cluster: {name: mydb}
  volumeSnapshotClassName: csi-snapclass
```

```
NAME             CLUSTER   METHOD           PHASE       AGE
before-upgrade   mydb      volumeSnapshot   Completed   48s
```

The `VolumeSnapshot` is named after the `Backup`, carries an owner reference to
it, and is deleted with it. `status.snapshotName`, `startedAt` and
`completedAt` record what happened; `status.error` explains a `Failed` phase.

## On a schedule

```yaml
apiVersion: database.serenedb.com/v1alpha1
kind: ScheduledBackup
metadata:
  name: nightly
spec:
  cluster: {name: mydb}
  schedule: "0 2 * * *"
  immediate: true
  keep: 7
  volumeSnapshotClassName: csi-snapclass
```

`schedule` is a five-field cron expression evaluated in UTC. `immediate` also
creates one `Backup` right after the schedule is created. `keep` deletes older
completed backups from this schedule beyond that count; leave it out to keep
everything. `suspend: true` pauses new backups without deleting the schedule.

Backups created by a schedule are named `<schedule>-<timestamp>` and labelled
`database.serenedb.com/scheduled-backup=<schedule>`. They do not carry an owner
reference, so deleting the schedule keeps its backups. The status shows
`lastScheduleTime`, `nextScheduleTime` and `lastBackup`; an unparsable
`schedule` reports `InvalidSchedule` in the `Ready` condition.

If the operator was down when a run was due, the next reconcile takes one
catch-up backup and then returns to the schedule.

## Restore

Restoring means creating a new server whose data volume is seeded from a
snapshot:

```yaml
apiVersion: database.serenedb.com/v1alpha1
kind: SereneDB
metadata:
  name: mydb-restored
spec:
  bootstrap:
    volumeSnapshotName: nightly-20260924-020000
  auth:
    existingSecret: mydb
  persistence:
    size: 100Gi
```

`bootstrap.volumeSnapshotName` becomes the `dataSource` of the volume claim
template, so the CSI driver provisions the new PVC from the snapshot. The field
is immutable and applies only when the volume is first created. Point
`auth.existingSecret` at the original server's password Secret, because the
restored data directory keeps the password it was initialized with; a freshly
generated Secret would not match. The requested `persistence.size` must be at
least the snapshot's size.

## What a snapshot is and is not

Snapshots are crash-consistent as of the moment after `CHECKPOINT`, which
flushes the write-ahead log to the data files. They carry everything on the
volume: databases, roles, persistent server secrets, indexes. There is no
point-in-time recovery and no incremental backup.

A logical export to object storage is not offered. `EXPORT DATABASE` in
SereneDB 26.09.2 writes the table files and then stops before `schema.sql`,
so its output cannot be imported; see
[serenedb/serenedb#1227](https://github.com/serenedb/serenedb/issues/1227).
