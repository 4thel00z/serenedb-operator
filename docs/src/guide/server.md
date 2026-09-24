# Running a server

A `SereneDB` describes one server. This chapter walks the spec top to bottom;
the [reference](../reference/serenedb.md) has every field with its default.

## Image

```yaml
spec:
  image:
    repository: serenedb/serenedb
    tag: "26.09.2"
    pullPolicy: IfNotPresent
  imagePullSecrets: [{name: registry-credentials}]
```

Pin a release tag. With `IfNotPresent` a `latest` tag is never re-pulled, and a
change of tag is what rolls the pod. Changing the tag is the upgrade procedure,
see [Upgrades and configuration changes](./upgrades.md).

## Persistence

```yaml
spec:
  persistence:
    size: 100Gi
    storageClassName: fast-ssd
    accessModes: [ReadWriteOnce]
    retentionPolicy:
      whenDeleted: Retain
      whenScaled: Retain
```

The data directory lives on a PersistentVolumeClaim named `data-<name>-0`,
created from the StatefulSet's claim template. Size, storage class, access modes
and `existingClaim` are immutable after creation, because StatefulSet claim
templates cannot change; the API server rejects such updates and names the
field. To grow a volume, expand the PVC directly if the storage class allows it.

`existingClaim` mounts a PVC you created yourself instead. `retentionPolicy`
decides what happens to the claim when the `SereneDB` is deleted. `Retain`, the
default, keeps the data and the generated password Secret; `Delete` removes both.

## Resources and threads

```yaml
spec:
  resources:
    requests: {cpu: "4", memory: 16Gi}
    limits: {cpu: "4", memory: 16Gi}
  config:
    cpuThreads: 4
    ioThreads: 2
```

A recommended production start is requests equal to limits, at least 2 CPU and
4Gi. The server's thread auto-detection sees the node's cores, not the pod's
CPU limit, so set `cpuThreads`, `ioThreads` and `backgroundThreads` explicitly
whenever the limit is small relative to the node.

## Service

```yaml
spec:
  service:
    type: LoadBalancer
    annotations:
      service.beta.kubernetes.io/aws-load-balancer-type: nlb
```

The client Service is named after the `SereneDB` and exposes the pg-wire port,
plus the HTTP port when that listener is on. `ClusterIP` is the default;
`NodePort` accepts a pinned `nodePort`, and an auto-assigned node port survives
reconciles. A second, headless Service named `<name>-hl` governs the StatefulSet
and is not meant for clients.

## Scheduling and pod settings

`nodeSelector`, `tolerations`, `affinity`, `priorityClassName`, `podLabels` and
`podAnnotations` pass straight through to the pod. `env` adds environment
variables to the server container, for example `NUMA=disable` to turn off the
image entrypoint's NUMA interleaving.

`terminationGracePeriodSeconds` defaults to 120. Shutdown checkpoints and drains
background and search tasks, so raise it for large data directories.

## Security posture

The pod runs the upstream image unchanged with the same hardening as the
upstream Helm chart: uid 999, `runAsNonRoot`, all capabilities dropped, the
runtime default seccomp profile, `fsGroup: 0` because the image's data directory
is group-0 writable, and no service account token mounted. The uid is pinned
because the image declares its user by name and the kubelet cannot verify
`runAsNonRoot` against a name; without the pin the pod never starts.

`networkPolicy.enabled: true` adds a NetworkPolicy that admits ingress only on
the serving ports. Combine it with your own policies for source restrictions.
