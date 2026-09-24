# Differences from the Helm chart

The operator renders the objects the upstream chart in `packages/helm/serenedb`
renders, with the same hardening and probes, and most values keep their names as
spec fields. These differ:

| Chart value | Operator |
|---|---|
| `auth.password` | Dropped. Put the password in a Secret and set `auth.existingSecret` |
| `tls.existingSecret` | `tls.secretName` |
| `persistence.storageClass` | `persistence.storageClassName` |
| `extraEnv` | `env` |
| `initContainers`, `sidecars`, `extraVolumes`, `extraVolumeMounts`, `extraObjects` | Not supported |
| `readinessProbe`, `livenessProbe`, `startupProbe`, `updateStrategy`, `podSecurityContext`, `containerSecurityContext` | Fixed to the chart defaults |
| `containerSecurityContext.runAsUser` | Pinned to `999`. The chart omits it, which keeps its pod from starting under `runAsNonRoot`; see [serenedb/serenedb#1228](https://github.com/serenedb/serenedb/pull/1228) |

Beyond the chart, the operator adds the `Database`, `DatabaseRole`,
`ServerSecret`, `Backup` and `ScheduledBackup` kinds, status conditions, and
the `bootstrap` restore field.
