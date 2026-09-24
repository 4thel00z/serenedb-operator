# Databases and roles

`Database` and `DatabaseRole` manage objects inside a running server over its
pg-wire port. The operator connects as `postgres` with the password from the
server's Secret, so nothing else needs credentials. Both name their server in
`spec.cluster.name`, wait until that `SereneDB` is `Ready`, and report an
`Applied` reason in their own `Ready` condition. They are re-applied every ten
minutes and whenever the `SereneDB` or a referenced Secret changes.

## Database

```yaml
apiVersion: database.serenedb.com/v1alpha1
kind: Database
metadata:
  name: app-production
spec:
  cluster: {name: mydb}
  name: app_production
  reclaimPolicy: Retain
```

The operator checks `pg_database` and runs `CREATE DATABASE IF NOT EXISTS` when
the database is missing. `name` defaults to the object name and is immutable.
SereneDB's `CREATE DATABASE` has no `OWNER` clause, so ownership is set with
SQL after creation if you need it.

`reclaimPolicy: Delete` drops the database when the object is deleted. The
default `Retain` leaves it on the server.

## DatabaseRole

```yaml
apiVersion: database.serenedb.com/v1alpha1
kind: DatabaseRole
metadata:
  name: app-rw
spec:
  cluster: {name: mydb}
  name: app_rw
  login: true
  createDB: false
  passwordSecret: {name: app-rw-password, key: password}
  inRoles: [readers]
  validUntil: "2030-01-01"
  reclaimPolicy: Delete
```

Roles are global to the server. On every pass the operator checks `pg_roles`,
then runs one `CREATE ROLE` or one `ALTER ROLE` carrying every attribute:
`LOGIN`, `SUPERUSER`, `CREATEDB`, `CREATEROLE`, `INHERIT`, `CONNECTION LIMIT`
and `VALID UNTIL`, each in its positive or negative form. A role you edit by
hand converges back to the spec within ten minutes.

`inRoles` is the complete list of memberships. The operator reads
`pg_auth_members`, grants what is missing and revokes what is no longer listed.
Roles named in `inRoles` must already exist; create them as `DatabaseRole`
objects first.

The password is applied on creation and re-applied only when the Secret's
`resourceVersion` changes, because the server stores a SCRAM verifier and the
plaintext cannot be compared. Rotate a password by updating the Secret.

`DROP ROLE` fails while the role owns objects. The operator reports that as an
`SQLError` reason on the deleting object instead of looping; reassign or drop
the objects, and the deletion completes on the next attempt.

## Quoting and safety

Every name and value from a spec is quoted before it reaches SQL: identifiers
with double quotes, values as string literals. A name like `x"; DROP ROLE
postgres; --` is created as a role with that literal name.

## When the server is gone

If the `SereneDB` has been deleted or is being deleted, deleting a dependent
object skips the server-side drop and just completes, so cleanup never hangs on
a server that no longer exists.
