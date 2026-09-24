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
