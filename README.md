<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/logo-lockup-dark.svg">
  <img alt="SereneDB Operator" src="assets/logo-lockup.svg" width="560">
</picture>

# SereneDB Operator

A Kubernetes operator for [SereneDB](https://github.com/serenedb/serenedb), the Postgres-compatible
search and analytics database. You declare a `SereneDB` resource; the operator runs a single-node
server with a persistent data volume, generated superuser credentials, a flagfile ConfigMap,
client and headless Services, and reports readiness in the resource status.

SereneDB is a single-machine database, so a `SereneDB` always runs exactly one pod. Config and
image changes restart that pod; expect a short downtime window while it checkpoints and reloads
its indexes. Clients should reconnect on connection loss.

## Install

Requires Kubernetes 1.27 or newer.

```sh
make install                                      # CRDs
make deploy IMG=ghcr.io/4thel00z/serenedb-operator:latest
```

Or render one installer manifest:

```sh
make build-installer IMG=<registry>/serenedb-operator:<tag>
kubectl apply -f dist/install.yaml
```

## Create a database

```yaml
apiVersion: database.serenedb.com/v1alpha1
kind: SereneDB
metadata:
  name: mydb
spec:
  image:
    tag: "26.09.2"
  persistence:
    size: 50Gi
  resources:
    requests: {cpu: "2", memory: 4Gi}
    limits: {cpu: "2", memory: 4Gi}
  config:
    cpuThreads: 2
    ioThreads: 2
```

```sh
kubectl apply -f mydb.yaml
kubectl get serenedb mydb
NAME   READY   VERSION   AGE
mydb   True    26.09.2   2m
```

Connect from inside the cluster with the generated password:

```sh
PGPASSWORD=$(kubectl get secret mydb -o jsonpath='{.data.postgres-password}' | base64 -d)
psql -h mydb.<namespace>.svc -p 7890 -U postgres -d postgres
```

`.status.host`, `.status.port` and `.status.secretName` hold the same connection details.

## Passwords

The operator passes the password to `serened` as `POSTGRES_PASSWORD`, which the server honors
only on the first boot with an empty data directory. Change it later in SQL with
`ALTER ROLE postgres PASSWORD '...'`, not through the resource.

When `auth.existingSecret` is empty the operator generates a Secret named after the `SereneDB`
and never rewrites it. Its lifetime follows the data volume: with the default
`persistence.retentionPolicy.whenDeleted: Retain` the Secret is left behind together with the
PVC, so recreating the `SereneDB` finds the password the data directory was initialized with.
With `whenDeleted: Delete` both are garbage collected.

To bring your own password, create a Secret with key `postgres-password` (or set
`auth.passwordKey`) and reference it in `auth.existingSecret`. The database is not created until
the Secret and key exist; the `Ready` condition reports `SecretMissing` or `SecretKeyMissing`
until then.

## TLS

```yaml
spec:
  tls:
    enabled: true
    secretName: mydb-tls     # kubernetes.io/tls Secret, cert-manager compatible
    minVersion: "1.3"
  listeners:
    postgres:
      sslmode: require
```

When TLS is on, the optional Elasticsearch-compatible HTTP listener (`listeners.http.enabled`)
serves HTTPS.

## Spec reference

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
| `config.cpuThreads` / `ioThreads` / `backgroundThreads` | `0` | 0 auto-detects from the node, not the pod limit. Set them explicitly with small CPU limits |
| `config.authTimeout` | `30s` | |
| `config.idleSessionTimeout` | `75s` | |
| `config.pgMaxMessageBytes` | `0` | 0 keeps the built-in default |
| `config.hba` | engine default | `pg_hba.conf` content, verbatim |
| `config.extraFlags` | `[]` | Extra `--flag=value` lines appended to the flagfile |
| `persistence.size` | `20Gi` | Immutable after creation |
| `persistence.storageClassName` | cluster default | Immutable |
| `persistence.accessModes` | `[ReadWriteOnce]` | Immutable |
| `persistence.existingClaim` | | Use a pre-created PVC instead. Immutable |
| `persistence.retentionPolicy.whenDeleted` | `Retain` | `Retain` or `Delete`. Also decides the generated Secret's fate |
| `persistence.retentionPolicy.whenScaled` | `Retain` | |
| `resources` | `{}` | Recommended start: requests equal limits, at least 2 CPU and 4Gi |
| `terminationGracePeriodSeconds` | `120` | Shutdown budget for checkpointing. Raise it for large data directories |
| `service.type` | `ClusterIP` | `ClusterIP`, `NodePort` or `LoadBalancer` |
| `service.nodePort` | auto | Pin the node port when `service.type` is `NodePort` |
| `service.annotations` | `{}` | |
| `networkPolicy.enabled` | `false` | Restrict ingress to the serving ports |
| `env` | `[]` | Extra container environment, for example `NUMA=disable` |
| `podAnnotations`, `podLabels`, `nodeSelector`, `tolerations`, `affinity`, `priorityClassName` | | Standard passthroughs |

Volume claim fields are immutable because StatefulSet claim templates cannot change after
creation; the API server rejects such updates with a message naming the field.

## Status

| Field | Meaning |
|---|---|
| `conditions[Ready]` | `True` once the pod passes its readiness probe and the rollout is complete. Reasons: `PodReady`, `PodNotReady`, `SecretMissing`, `SecretKeyMissing`, `TLSSecretNameRequired`, `TLSSecretMissing`, `ReconcileError` |
| `conditions[Progressing]` | `True` while a rollout is in flight |
| `readyReplicas` | 0 or 1 |
| `version` | Image tag of the last completed rollout |
| `host`, `port`, `secretName` | Connection details |

## What the operator creates

For a `SereneDB` named `mydb`:

| Object | Name | Notes |
|---|---|---|
| StatefulSet | `mydb` | 1 replica, `pg_isready` probes, runs as uid 999 with all capabilities dropped, no service account token |
| ConfigMap | `mydb` | `serened.conf` flagfile, plus `pg_hba.conf` when `config.hba` is set. A hash on the pod template restarts the pod when it changes |
| Secret | `mydb` | Generated password, only when `auth.existingSecret` is empty |
| Service | `mydb` | Client entry point |
| Service | `mydb-hl` | Headless, governs the StatefulSet |
| NetworkPolicy | `mydb` | Only when `networkPolicy.enabled` |

The pod runs the upstream image unchanged: the ConfigMap is mounted at `/etc/serenedb` and the
image entrypoint appends `--flagfile` itself. The rendered objects follow the upstream Helm chart
in `packages/helm/serenedb`, and most chart values keep their names as spec fields.

### Differences from the Helm chart

| Chart value | Operator |
|---|---|
| `auth.password` | Dropped. Put the password in a Secret and set `auth.existingSecret` |
| `tls.existingSecret` | `tls.secretName` |
| `persistence.storageClass` | `persistence.storageClassName` |
| `extraEnv` | `env` |
| `initContainers`, `sidecars`, `extraVolumes`, `extraVolumeMounts`, `extraObjects` | Not supported |
| `readinessProbe`, `livenessProbe`, `startupProbe`, `updateStrategy`, `podSecurityContext`, `containerSecurityContext` | Fixed to the chart defaults |
| `containerSecurityContext.runAsUser` | Pinned to `999`, the uid of the image's `serenedb` user. The image declares that user by name and the kubelet cannot verify `runAsNonRoot` against a name, so without the pin the pod never starts. A custom image must use the same uid |

## Development

```sh
make test          # unit tests for the manifest builders and envtest reconciler tests
make lint
make test-e2e      # kind cluster: deploys the operator, runs SELECT 1 against a real SereneDB
make run           # run the operator against the current kubeconfig
```

`make test-e2e` builds the operator image, loads it into a kind cluster named
`serenedb-operator-test-e2e`, creates a `SereneDB`, waits for `Ready`, runs queries through the
Service with the generated password, rolls the pod through a config change, checks the data
survived, and verifies garbage collection on delete. Set `E2E_PREBUILT_IMAGE=1` to skip the
in-suite image build when a pipeline has already built and loaded
`ghcr.io/4thel00z/serenedb-operator:e2e` into the cluster.

## License

Apache 2.0, the same as SereneDB.
