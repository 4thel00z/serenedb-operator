# TLS and listeners

## TLS

```yaml
spec:
  tls:
    enabled: true
    secretName: mydb-tls
    minVersion: "1.3"
  listeners:
    postgres:
      sslmode: require
```

`secretName` is a `kubernetes.io/tls` Secret with `tls.crt` and `tls.key`, the
shape cert-manager produces, so a `Certificate` pointing at `mydb-tls` is all
the automation needed. The Secret is mounted at `/etc/serenedb-tls` and the
flagfile gains `--tls_cert`, `--tls_key` and `--tls_min_version`.

Until the Secret exists, the `SereneDB` reports `TLSSecretMissing` and the
StatefulSet is not created. Enabling TLS without a `secretName` reports
`TLSSecretNameRequired`.

`listeners.postgres.sslmode` is appended to the listener URL as `?sslmode=`,
for example `require` to refuse plaintext clients. Empty keeps the server
default.

## The pg-wire listener

```yaml
spec:
  listeners:
    postgres:
      port: 7890
```

The port is the container port, the Service port and what the probes check.
The default 7890 is what the SereneDB documentation and clients assume.

## The HTTP listener

```yaml
spec:
  listeners:
    http:
      enabled: true
      port: 9200
      corsOrigins: https://app.example.com
```

SereneDB can serve an Elasticsearch-compatible HTTP API. Enabling it adds a
second container port, a second Service port, and a second entry in the single
`--listen` flag the server takes. When TLS is on, this listener serves HTTPS.
`corsOrigins` passes through as `--http_cors_origins`.

The server reads all listeners from one comma-separated `--listen` value; a
repeated flag would be last-wins, which is why the operator renders them
together.
