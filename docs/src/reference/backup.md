# Backup

`backups.database.serenedb.com`, namespaced. One snapshot of a server's data
volume.

## Spec

| Field | Default | Description |
|---|---|---|
| `cluster.name` | required | The `SereneDB` in the same namespace |
| `method` | `volumeSnapshot` | The only method offered |
| `volumeSnapshotClassName` | cluster default | `VolumeSnapshotClass` to use |

## Status

| Field | Meaning |
|---|---|
| `phase` | `Pending`, `Running`, `Completed` or `Failed` |
| `startedAt` | When `CHECKPOINT` ran |
| `completedAt` | When the snapshot became ready to use |
| `snapshotName` | The `VolumeSnapshot`, named after the Backup |
| `error` | Why the phase is `Pending` or `Failed` |

## Behavior

Waits for the `SereneDB` to be `Ready`, runs `CHECKPOINT`, creates a
`VolumeSnapshot` of `data-<cluster>-0` (or the `existingClaim`) owned by the
Backup, then watches or polls it until `readyToUse` is true or `status.error`
is set. `Completed` and `Failed` are final. Deleting the Backup deletes the
snapshot.

Print columns: `CLUSTER`, `METHOD`, `PHASE`, `AGE`; `-o wide` adds `SNAPSHOT`.
