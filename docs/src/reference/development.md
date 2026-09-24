# Development

The operator is Go on controller-runtime, scaffolded with operator-sdk. The
module is `github.com/4thel00z/serenedb-operator`.

## Layout

| Path | Contents |
|---|---|
| `api/v1alpha1` | The six kinds and their shared types |
| `internal/resources` | Pure functions from a `SereneDB` to its Kubernetes objects |
| `internal/statements` | Pure functions rendering the SQL the operator runs |
| `internal/sqlexec` | The SQL session interface, the pgx adapter and a recording fake |
| `internal/controller` | One reconciler per kind plus the shared dependent helper |
| `config` | CRDs, RBAC, manager Deployment, samples, kustomize entry points |
| `test/e2e` | The kind suite |
| `docs` | This book |

## Make targets

```sh
make test          # generate, vet, unit tests, envtest reconciler tests
make lint          # golangci-lint
make test-e2e      # kind cluster: deploy, create a SereneDB, run SQL against it
make run           # run the operator against the current kubeconfig
make build-installer IMG=...   # dist/install.yaml
```

## Tests

Builders and statement renderers have plain unit tests. Reconcilers run under
envtest with a real API server, no kubelet and a fake SQL session that records
statements, so tests assert the exact quoted SQL; readiness is simulated by
patching the StatefulSet status. The VolumeSnapshot CRD in `test/crds` lets
backup tests create snapshots and set their status.

The kind suite covers `SereneDB`, `Database`, `DatabaseRole` and
`ServerSecret` against a real server: readiness, `SELECT 1` through the Service
with the generated password, wrong-password rejection, a config roll with data
intact, creating a database and a role, connecting as that role, dropping on
delete, and garbage collection. Backups are envtest-only, because kind ships no
CSI snapshotter.

`E2E_PREBUILT_IMAGE=1` skips the in-suite image build when a pipeline has
already built and loaded `ghcr.io/4thel00z/serenedb-operator:e2e` into the
cluster.

## Documentation

```sh
mdbook serve docs --open
```

The book publishes to GitHub Pages from the `book` workflow on every push to
`main`.
