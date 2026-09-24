# Upgrades and configuration changes

## What rolls the pod

Changing `image.tag` or `image.repository`, anything under `config`,
`listeners` or `tls`, `resources`, `env`, scheduling fields or
`terminationGracePeriodSeconds` updates the StatefulSet's pod template, and the
StatefulSet controller recreates the single pod. Configuration lives in a
ConfigMap; its content is hashed into a pod template annotation, so a flagfile
change rolls the pod even though the ConfigMap name stays the same.

During the roll the `SereneDB` reports `Ready: False` with reason `PodNotReady`
and `Progressing: True` with reason `RollingOut`, and `status.version` keeps the
previous tag until the new pod is serving.

## What does not roll the pod

Changes to `service`, `networkPolicy`, `podLabels` on the StatefulSet's own
metadata, and `auth` are applied without a restart. `auth` changes are also
without effect on an initialized server, see [Passwords](./passwords.md).

## What cannot change

`persistence.size`, `storageClassName`, `accessModes`, `existingClaim` and
`bootstrap` are immutable. The API server rejects the update with a message
naming the field. To move to a bigger volume, expand the PVC in place if the
storage class allows it, or take a [backup](./backups.md) and restore into a
new `SereneDB` with the size you want.

## Upgrading SereneDB

```sh
kubectl patch serenedb mydb --type merge -p '{"spec":{"image":{"tag":"26.10.1"}}}'
```

The new pod opens the same data directory. Check the SereneDB release notes for
data format changes before crossing a major version, and take a `Backup`
first. Downgrades are not guaranteed by the database.

## Downtime

Each roll is one graceful shutdown plus one startup. Shutdown checkpoints and
drains background tasks within `terminationGracePeriodSeconds`; startup binds
the pg-wire port only after indexes are loaded, and the startup probe allows
five minutes for that. Clients see connection errors in between and should
reconnect.

## Upgrading the operator

Deploy the new operator image and re-apply the CRDs:

```sh
make install
make deploy IMG=ghcr.io/4thel00z/serenedb-operator:<tag>
```

The operator only patches mutable StatefulSet fields, so a new operator version
that renders the same pod template leaves existing servers untouched, and one
that renders a different template rolls them once.
