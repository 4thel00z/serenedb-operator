# Installation

The operator needs Kubernetes 1.27 or newer. Backups additionally need a CSI
driver with snapshot support and the snapshot controller, see
[Backups and restore](./guide/backups.md).

## From a release

Every release ships one installer manifest with the CRDs and the operator
Deployment, pinned to that release's image on GitHub Container Registry.

```sh
kubectl apply -f https://github.com/4thel00z/serenedb-operator/releases/latest/download/install.yaml
```

To pin a version, use that release's own download path:

```sh
kubectl apply -f https://github.com/4thel00z/serenedb-operator/releases/download/v0.1.0/install.yaml
```

The images are published for `linux/amd64` and `linux/arm64` as
`ghcr.io/4thel00z/serenedb-operator:<tag>`.

## From the repository

```sh
git clone https://github.com/4thel00z/serenedb-operator
cd serenedb-operator
make install                                              # CRDs
make deploy IMG=ghcr.io/4thel00z/serenedb-operator:latest # operator
```

`make deploy` renders the kustomize configuration under `config/default` and
applies it: the `serenedb-operator-system` namespace, a Deployment with one
replica, the ClusterRole the controllers need, and a metrics Service.

## One installer manifest

For GitOps or an air-gapped cluster, render everything into a single file and
apply that:

```sh
make build-installer IMG=<registry>/serenedb-operator:<tag>
kubectl apply -f dist/install.yaml
```

## Building the image

```sh
make docker-build docker-push IMG=<registry>/serenedb-operator:<tag>
```

The Dockerfile builds a static Go binary and copies it onto
`gcr.io/distroless/static:nonroot`.

## Verifying

```sh
kubectl get pods -n serenedb-operator-system
kubectl get crd | grep serenedb.com
```

Six CRDs are installed: `serenedbs`, `databases`, `databaseroles`,
`serversecrets`, `backups` and `scheduledbackups`, all in
`database.serenedb.com`. The short name `sdb` works for `serenedbs`.

## Uninstalling

```sh
make undeploy
make uninstall
```

Uninstalling the CRDs deletes every `SereneDB` and, through owner references,
its StatefulSet and Services. Data volumes follow each SereneDB's
`persistence.retentionPolicy`, which is `Retain` by default, so PVCs and the
generated password Secrets stay behind until you delete them yourself.
