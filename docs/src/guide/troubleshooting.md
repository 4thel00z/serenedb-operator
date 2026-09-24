# Troubleshooting

Every kind reports a `Ready` condition whose `reason` names the situation and
whose `message` gives the detail. Start there:

```sh
kubectl describe serenedb mydb
kubectl get databaserole app-rw -o jsonpath='{.status.conditions[0]}'
kubectl logs -n serenedb-operator-system deploy/serenedb-operator-controller-manager
```

## SereneDB

| Reason | Meaning | What to do |
|---|---|---|
| `PodNotReady` | The pod is starting or restarting | Wait; check `kubectl describe pod mydb-0` if it persists |
| `SecretMissing`, `SecretKeyMissing` | `auth.existingSecret` or its key does not exist | Create the Secret with the right key |
| `TLSSecretNameRequired`, `TLSSecretMissing` | TLS is on without a Secret | Set `tls.secretName` and create the Secret |
| `ReconcileError` | An API call failed | Read the message; the operator retries with backoff |

`Progressing: True` with `RollingOut` is normal during any change that rolls the
pod.

## Pod stuck in CreateContainerConfigError

`kubectl describe pod` shows the cause. A missing password Secret key shows up
here when the Secret was deleted after the StatefulSet was created. The message
"container has runAsNonRoot and image has non-numeric user" means the container
security context lost its `runAsUser`, which the operator always sets; it
points at a mutating webhook or policy engine rewriting the pod.

## Pod never becomes ready

The startup probe waits up to five minutes for the pg-wire port. Large data
directories take longer to load their indexes; if the pod is repeatedly killed
by the probe, look at the container logs for the loading progress. Memory
limits far below the working set show up as `OOMKilled` restarts.

## Databases, roles and server secrets

| Reason | Meaning |
|---|---|
| `Applied` | The object exists on the server as specified |
| `ClusterNotFound`, `ClusterNotReady` | Waiting for the `SereneDB` named in `spec.cluster` |
| `ConnectionFailed` | The operator could not connect; retried every 30 seconds |
| `SQLError` | The server rejected a statement; the message carries the server's error |
| `PasswordSecretMissing`, `ValuesSecretMissing` | A referenced Kubernetes Secret or key is absent |
| `InvalidSpec` | A value cannot be rendered into SQL, such as a bad option name |

A `DatabaseRole` deletion that stays pending with `SQLError` usually means the
role still owns objects; `DROP ROLE` refuses until they are reassigned or
dropped.

`ConnectionFailed` on every dependent at once, while the `SereneDB` is `Ready`,
points at a NetworkPolicy between the operator namespace and the database
namespace, or at TLS: with `tls.enabled` the operator connects with
`sslmode=require` and does not verify the certificate.

## Backups

| Phase | Meaning |
|---|---|
| `Pending` | Waiting for the server; `status.error` says why |
| `Running` | Snapshot requested, not yet ready to use |
| `Completed` | `status.snapshotName` is ready to use |
| `Failed` | `status.error` holds the snapshot error or the missing-API message |

"the cluster has no VolumeSnapshot API" means the snapshot CRDs and controller
are not installed. A snapshot that stays `Running` for a long time is usually
the CSI driver's problem; `kubectl describe volumesnapshot <name>` shows its
events.

## Events and logs

The operator logs each precondition it waits on at info level with the reason
and message. Kubernetes events on the pod, the PVC and the VolumeSnapshot carry
the scheduler's, kubelet's and CSI driver's side of the story.
