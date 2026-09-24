# Quickstart

This walk-through creates a server, connects to it, and adds a database and a
role. It assumes the operator is [installed](./installation.md).

## Create a server

```yaml
apiVersion: database.serenedb.com/v1alpha1
kind: SereneDB
metadata:
  name: mydb
spec:
  image:
    tag: "26.09.2"
  persistence:
    size: 20Gi
  resources:
    requests: {cpu: "2", memory: 4Gi}
    limits: {cpu: "2", memory: 4Gi}
  config:
    cpuThreads: 2
    ioThreads: 2
```

```sh
kubectl apply -f mydb.yaml
kubectl get serenedb mydb -w
```

```
NAME   READY   VERSION   AGE
mydb   False             5s
mydb   True    26.09.2   40s
```

`Ready` turns `True` once the pod passes its readiness probe and the rollout is
complete. The pg-wire listener binds only after the server has loaded its
indexes, so on a large data directory this takes longer than on a fresh one.

## Connect

The operator generated a Secret named after the server with the `postgres`
password. Connection details are also in the status:

```sh
kubectl get serenedb mydb -o jsonpath='{.status.host}:{.status.port}{"\n"}'
PGPASSWORD=$(kubectl get secret mydb -o jsonpath='{.data.postgres-password}' | base64 -d)
kubectl port-forward svc/mydb 7890:7890 &
psql -h 127.0.0.1 -p 7890 -U postgres -d postgres -c 'SELECT version()'
```

Inside the cluster, connect to `mydb.<namespace>.svc` on port 7890.

## Add a database and a role

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: app-rw-password
stringData:
  password: change-me
---
apiVersion: database.serenedb.com/v1alpha1
kind: Database
metadata:
  name: app
spec:
  cluster: {name: mydb}
  name: app_production
---
apiVersion: database.serenedb.com/v1alpha1
kind: DatabaseRole
metadata:
  name: app-rw
spec:
  cluster: {name: mydb}
  name: app_rw
  login: true
  passwordSecret: {name: app-rw-password}
```

```sh
kubectl apply -f app.yaml
kubectl get database,databaserole
```

```
NAME                               CLUSTER   READY   AGE
database.database.serenedb.com/app mydb      True    3s

NAME                                      CLUSTER   READY   AGE
databaserole.database.serenedb.com/app-rw mydb      True    3s
```

Both objects wait for the server to be `Ready`, connect as the superuser, and
run the SQL that makes them exist. The role can now log in:

```sh
PGPASSWORD=change-me psql -h 127.0.0.1 -p 7890 -U app_rw -d app_production
```

## Next

- [Running a server](./guide/server.md) covers the rest of the `SereneDB` spec.
- [Passwords](./guide/passwords.md) explains why the superuser password is a
  first-boot setting and how the Secret's lifetime follows the data volume.
- [Backups and restore](./guide/backups.md) sets up volume snapshots.
