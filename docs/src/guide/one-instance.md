# One instance instead of Postgres and Qdrant

A retrieval pipeline usually runs two stores: Postgres for documents, chunks
and run metadata, and Qdrant for the embeddings. SereneDB serves both from one
server. It speaks the Postgres wire protocol, so the relational tables move
over unchanged, and its inverted index covers exact filters, full-text search
with BM25 and approximate nearest neighbour search over fixed-size `FLOAT[N]`
vectors in a single index. One `SereneDB` object gives you one volume, one
password Secret, one backup and one connection string.

The sample under `config/samples/one-instance` is the complete setup. The
schema and the retrieval queries on this page were run against SereneDB
26.09.2 on a kind cluster. The `ai_embed` calls need a reachable embeddings
provider and were not.

## Deploy

```sh
kubectl create namespace vectors
kubectl apply -k config/samples/one-instance -n vectors
kubectl -n vectors wait serenedb/store --for=condition=Ready --timeout=5m
```

The bundle creates four objects:

| Object | Kind | Purpose |
|---|---|---|
| `store` | `SereneDB` | The server, pinned to `26.09.2`, 2 CPU, 4Gi, 50Gi volume |
| `app` | `Database` | The application database |
| `app` | `DatabaseRole` | Login role, password from Secret `app-password` |
| `embeddings` | `ServerSecret` | `openai` type credentials for `ai_embed`, from Secret `embeddings-api` |

Replace the two Secret values before applying to anything but a test cluster.
The `openai` type names the wire protocol, not the vendor: any embeddings
endpoint compatible with the OpenAI API works when `BASE_URL` points at it.

## Schema

The role that owns the tables must also own the search index, because only
the owner can refresh it. So the superuser grants the `app` role the right to
create in the schema, and `app` creates everything else. The host below
resolves inside the cluster, so run these from a pod or through
`kubectl -n vectors port-forward svc/store 7890` with `-h localhost`.

```sh
PGPASSWORD=$(kubectl -n vectors get secret store -o jsonpath='{.data.postgres-password}' | base64 -d) \
  psql -h store.vectors.svc -p 7890 -U postgres -d app -f config/samples/one-instance/grant.sql
PGPASSWORD=change-me \
  psql -h store.vectors.svc -p 7890 -U app -d app -f config/samples/one-instance/schema.sql
```

`schema.sql` is short enough to read in full:

```sql
CREATE TEXT SEARCH DICTIONARY english_stem AS
    split_text(case := 'lower') | stem_words('en_US.UTF-8')
    WITH (frequency, position);

CREATE TABLE documents (
    id UUID PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    title TEXT,
    source_uri TEXT,
    created_at TIMESTAMP DEFAULT now()
);

CREATE TABLE chunks (
    id UUID PRIMARY KEY,
    document_id UUID NOT NULL,
    tenant_id TEXT NOT NULL,
    workflow_run_id TEXT,
    position INTEGER,
    body TEXT,
    metadata JSON,
    emb FLOAT[1024]
);

CREATE INDEX chunks_search ON chunks USING inverted (
    tenant_id,
    workflow_run_id,
    body english_stem,
    emb ivf (metric = 'cosine')
) INCLUDE (document_id, position);
```

The dictionary lower-cases and stems English, so a query for `invoice` finds
`invoices`. In the index, a column without a dictionary is indexed verbatim
and matches exact values, which is what a filter on a tenant or a run id
needs. `body` gets the dictionary and BM25 scoring. `emb` gets an IVF vector
index with cosine distance. `INCLUDE` stores two more columns in the index so
a search returns them without touching the table.

## How Qdrant concepts map

| Qdrant | SereneDB |
|---|---|
| Collection, one per embedding model | Table with one `FLOAT[N]` column per model, `N` fixed by the model |
| Point id, often a hash of the source id | `id UUID PRIMARY KEY`, for example `md5(source_id)::UUID` |
| Payload fields used in filters | Verbatim columns in the inverted index |
| Payload fields only returned | Ordinary columns, or `INCLUDE` columns |
| Named dense vector | The `emb` column, `emb ivf (metric = 'cosine')` |
| Distance `Cosine`, `Euclid`, `Dot` | `metric = 'cosine'`, `'l2'`, `'ip'` with operators `<=>`, `<->`, `<#>` |
| `upsert` | `INSERT ... ON CONFLICT (id) DO UPDATE` |
| `search` with a filter | `WHERE col @@ 'value' ORDER BY emb <=> $q LIMIT k` |
| `delete` by payload filter | `DELETE FROM chunks WHERE workflow_run_id = 'run-7'` |
| HNSW index, `ef` | IVF index, `sdb_nprobe` session setting |
| Snapshot | `Backup`, which covers the tables and the vectors together |

## Writing

Write rows as you would into Postgres. The vector is a fixed-size array and
must be cast to the column's dimension.

```sql
INSERT INTO chunks
VALUES (md5('doc-7#0')::UUID, '5f1c...'::UUID, 'acme', 'run-7', 0,
        'invoices are due on friday', '{"page": 1}', $1::FLOAT[1024])
ON CONFLICT (id) DO UPDATE SET body = excluded.body, emb = excluded.emb;
```

The inverted index is refreshed by a background thread, so a row becomes
searchable shortly after the insert rather than at commit. After a bulk load,
or in a test that queries right after writing, refresh explicitly:

```sql
VACUUM (REFRESH_TABLE) chunks;
```

Run the refresh as its own statement after the write has committed. A
refresh inside the same transaction as the write still sees the old rows.
This is the one behavioural difference from Qdrant that shows up in test
suites. Plain relational reads of the table are consistent at commit as usual.

## Querying

Select from the index by name for filtered and full-text queries. A
k-nearest-neighbour search is an `ORDER BY` on the distance operator plus
`LIMIT`, and it uses the IVF index whether you select from the index or the
table.

Filtered nearest neighbours, the everyday retrieval query:

```sql
SELECT document_id, position, body
FROM chunks_search
WHERE tenant_id @@ 'acme'
ORDER BY emb <=> $1::FLOAT[1024]
LIMIT 10;
```

Full-text with relevance ranking:

```sql
SELECT body, bm25(chunks_search.tableoid) AS score
FROM chunks_search
WHERE tenant_id @@ 'acme' AND body @@ 'invoice'
ORDER BY score DESC
LIMIT 10;
```

Hybrid search with reciprocal rank fusion, which ranks each branch on its own
scale and sums `1 / (60 + rank)`:

```sql
WITH lexical AS (
    SELECT id, row_number() OVER (ORDER BY bm25(chunks_search.tableoid) DESC) AS r
    FROM chunks_search
    WHERE body @@ 'invoice'
    ORDER BY r
    LIMIT 20
), semantic AS (
    SELECT id, row_number() OVER (ORDER BY emb <=> $1::FLOAT[1024]) AS r
    FROM chunks_search
    ORDER BY emb <=> $1::FLOAT[1024]
    LIMIT 20
)
SELECT c.body, sum(1.0 / (60 + r)) AS score
FROM (SELECT * FROM lexical UNION ALL SELECT * FROM semantic) u
JOIN chunks c USING (id)
GROUP BY c.body
ORDER BY score DESC
LIMIT 10;
```

Removing a run's vectors is a plain delete, followed by a refresh in a
separate transaction:

```sql
DELETE FROM chunks WHERE workflow_run_id = 'run-7';
```

```sql
VACUUM (REFRESH_TABLE) chunks;
```

## Embedding inside the database

With the `embeddings` ServerSecret in place, the server can call the
embeddings endpoint itself. `ai_embed` returns a variable-length `FLOAT[]`
whose length is fixed by the model, so cast it to the column's dimension and
pick a model that produces exactly that many. The schema uses 1024, which
`mxbai-embed-large` returns; `text-embedding-3-large` returns 3072 and would
need a `FLOAT[3072]` column instead.

```sql
UPDATE chunks
SET emb = ai_embed(body, 'mxbai-embed-large', 'embeddings')::FLOAT[1024]
WHERE emb IS NULL;

SELECT body FROM chunks_search
ORDER BY emb <=> ai_embed('when are invoices due', 'mxbai-embed-large', 'embeddings')::FLOAT[1024]
LIMIT 5;
```

Each call is a network request, so embed at write time and only embed the
query text at search time. The model must be the same on both sides.

## What to know before switching

- **Single node.** The operator runs one instance. Qdrant's sharding and
  replication have no counterpart, and neither does Postgres streaming
  replication. Size the volume and the resources for the whole workload.
- **IVF, not HNSW.** The vector index partitions vectors into clusters and
  scans the nearest `sdb_nprobe` of them, default 8. Raise the setting per
  session for recall, lower it for latency. Quantization with `quant` is
  available for `l2` and `ip` metrics only.
- **Fixed dimension.** `FLOAT[]` without a size is rejected by the index. A
  new model with a different dimension means a new column and a new index
  entry, which mirrors a new Qdrant collection.
- **Indexed column types.** `UUID`, `DECIMAL`, `HUGEINT` and `INTERVAL`
  cannot be listed in the inverted index. A `UUID` primary key is fine as the
  row identity, so keep it out of the column list.
- **Eventual visibility in the index.** Refresh with `VACUUM (REFRESH_TABLE)`
  when a query must see a write immediately.
- **Privileges are enforced.** A role sees nothing it was not granted. The
  owner of a table is the only role that can refresh its index, which is why
  the sample makes `app` the owner.
- **Backups are whole-volume.** A `Backup` snapshots the data volume, so the
  tables and the vectors are always restored to the same point.
