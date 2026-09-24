# ScheduledBackup

`scheduledbackups.database.serenedb.com`, namespaced. Backups on a cron
schedule.

## Spec

| Field | Default | Description |
|---|---|---|
| `cluster.name` | required | The `SereneDB` in the same namespace |
| `schedule` | required | Five-field cron expression, UTC |
| `suspend` | `false` | Stop creating new Backups |
| `immediate` | `false` | Create one Backup right after creation |
| `keep` | unset | Number of completed Backups from this schedule to retain |
| `method` | `volumeSnapshot` | Copied into each Backup |
| `volumeSnapshotClassName` | cluster default | Copied into each Backup |

## Status

| Field | Meaning |
|---|---|
| `conditions[Ready]` | `Scheduled`, `Suspended` or `InvalidSchedule` |
| `lastScheduleTime` | When the most recent Backup was created |
| `nextScheduleTime` | When the next one is due |
| `lastBackup` | Name of the most recent Backup |

## Behavior

Creates `<name>-<UTC timestamp>` Backups labelled
`database.serenedb.com/scheduled-backup=<name>`, without an owner reference.
One catch-up Backup is taken when a run was missed. With `keep`, completed
Backups beyond that count are deleted, oldest first by creation time.

Print columns: `CLUSTER`, `SCHEDULE`, `SUSPENDED`, `LAST`, `AGE`.
