# SereneDB

`serenedbs.database.serenedb.com`, short name `sdb`, namespaced. One server.

## Spec

| Field | Default | Description |
|---|---|---|
| `image.repository` | `serenedb/serenedb` | Server image repository |
| `image.tag` | `26.09.2` | Server image tag. Pin a release for production |
| `image.pullPolicy` | `IfNotPresent` | |
| `imagePullSecrets` | `[]` | |
| `auth.existingSecret` | generated | Secret holding the initial `postgres` password |
| `auth.passwordKey` | `postgres-password` | Key inside that Secret |
| `listeners.postgres.port` | `7890` | pg-wire port |
| `listeners.postgres.sslmode` | server default | `?sslmode=` listener parameter |
| `listeners.http.enabled` | `false` | Elasticsearch-compatible HTTP listener |
| `listeners.http.port` | `9200` | |
| `listeners.http.corsOrigins` | | `--http_cors_origins` |
| `tls.enabled` | `false` | Requires `tls.secretName` |
| `tls.secretName` | | `kubernetes.io/tls` Secret with `tls.crt` and `tls.key` |
| `tls.minVersion` | `1.2` | `1.2` or `1.3` |
| `config.logLevel` | `info` | `trace`, `debug`, `info`, `warning`, `error`, `fatal` |
| `config.maxConnections` | `0` | 0 means unlimited |
| `config.cpuThreads`, `ioThreads`, `backgroundThreads` | `0` | 0 auto-detects from the node, not the pod limit |
| `config.authTimeout` | `30s` | |
| `config.idleSessionTimeout` | `75s` | |
| `config.pgMaxMessageBytes` | `0` | 0 keeps the built-in default |
| `config.hba` | engine default | `pg_hba.conf` content, verbatim |
| `config.extraFlags` | `[]` | Extra `--flag=value` lines appended to the flagfile |
| `persistence.size` | `20Gi` | Immutable |
| `persistence.storageClassName` | cluster default | Immutable |
| `persistence.accessModes` | `[ReadWriteOnce]` | Immutable |
| `persistence.existingClaim` | | Use a pre-created PVC. Immutable |
| `persistence.retentionPolicy.whenDeleted` | `Retain` | `Retain` or `Delete`. Also decides the generated Secret's fate |
| `persistence.retentionPolicy.whenScaled` | `Retain` | |
| `bootstrap.volumeSnapshotName` | | Seed the data volume from a VolumeSnapshot. Immutable |
| `resources` | `{}` | Recommended start: requests equal limits, at least 2 CPU and 4Gi |
| `terminationGracePeriodSeconds` | `120` | Shutdown budget for checkpointing |
| `service.type` | `ClusterIP` | `ClusterIP`, `NodePort` or `LoadBalancer` |
| `service.nodePort` | auto | Pin the node port when `service.type` is `NodePort` |
| `service.annotations` | `{}` | |
| `networkPolicy.enabled` | `false` | Restrict ingress to the serving ports |
| `env` | `[]` | Extra container environment variables |
| `podAnnotations`, `podLabels`, `nodeSelector`, `tolerations`, `affinity`, `priorityClassName` | | Pod passthroughs |

## Status

| Field | Meaning |
|---|---|
| `conditions[Ready]` | `True` once the pod passes readiness and the rollout is complete. Reasons: `PodReady`, `PodNotReady`, `SecretMissing`, `SecretKeyMissing`, `TLSSecretNameRequired`, `TLSSecretMissing`, `ReconcileError` |
| `conditions[Progressing]` | `True` while a rollout is in flight, reasons `RollingOut` and `Stable` |
| `observedGeneration` | Spec generation the status reflects |
| `readyReplicas` | 0 or 1 |
| `version` | Image tag of the last completed rollout |
| `host`, `port`, `secretName` | Connection details |

## Print columns

`kubectl get sdb` shows `READY`, `VERSION` and `AGE`; `-o wide` adds `HOST`.
