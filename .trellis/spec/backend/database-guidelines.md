# Database Guidelines

## Scenario: Persist knowledge documents, chunks, and pgvector embeddings

### 1. Scope / Trigger

- Trigger: changing upload deduplication, processing ownership, document status, chunk/vector replacement, PostgreSQL configuration, row models, or SQL under `migrations/`.
- Goal: preserve the `rag` schema, 1536-dimensional vectors, duplicate protection, and all-or-nothing replacement semantics.

### 2. Signatures

Application-owned repository boundary:

```go
type DocumentRepository interface {
    KnowledgeBaseExists(context.Context, string) (bool, error)
    ActiveDocumentHashExists(context.Context, string, string) (bool, error)
    CreateDocument(context.Context, domain.Document) error
    GetDocument(context.Context, string) (domain.Document, error)
    ListDocuments(context.Context, string, int, int) ([]domain.Document, int64, error)
    DeleteDocument(context.Context, string) error
    TryMarkProcessing(context.Context, string) (domain.Document, bool, error)
    ReplaceDocumentChunks(context.Context, domain.Document, []domain.Chunk, [][]float32) error
    MarkFailed(context.Context, domain.Document) error
}
```

Concrete constructors:

```go
func database.Open(ctx context.Context, cfg config.Database) (*sqlx.DB, error)
func postgres.NewRepository(db *sqlx.DB) (*postgres.Repository, error)
```

Required database shape:

```sql
CREATE SCHEMA rag;
embedding vector(1536) NOT NULL;
CREATE UNIQUE INDEX uk_knowledge_document_kb_hash_active
ON rag.t_knowledge_document (kb_id, content_hash)
WHERE deleted = 0 AND content_hash IS NOT NULL;
```

### 3. Contracts

- PostgreSQL is the relational database; vector storage uses pgvector.
- Database row structs live in `adapter/postgres` and carry `db` tags. Domain objects carry no `db` or `json` tags.
- Every row-to-domain conversion calls domain restoration/validation; every domain-to-row conversion serializes maps explicitly.
- Duplicate identity is active `kb_id + SHA-256(file bytes)`. The pre-check improves feedback; PostgreSQL error `23505` is the concurrency-safe backstop and maps to `domain.ErrDuplicate`.
- Processing ownership uses one conditional update. Zero updated rows means ownership was not acquired; do not parse or call Embedding.
- Parsing, chunking, Embedding, vector-count validation, and exact 1536-dimension validation happen before the transaction.
- `ReplaceDocumentChunks` uses one `sqlx.Tx` in this order:
  1. delete old vectors using old chunk IDs;
  2. physically delete old chunks;
  3. insert all new chunks;
  4. insert/upsert all new vectors with `$n::jsonb` and `$n::vector`;
  5. update the document to `completed` with chunk count/config/metadata;
  6. commit.
- Any return before a successful commit rolls back. Do not key rollback logic to a local `err` that can be shadowed by `:=`.
- Failure persistence receives an already transitioned `failed` aggregate and does not recreate domain rules in SQL code.
- `migrations/000001_rag_baseline.sql` is for a new empty database only. The service never runs migrations automatically.
- `migrations/000002_knowledge_ingestion_upgrade.sql` must remain non-destructive and preserve `vector(1536)`.
- Database env keys: `POSTGRESQL_DSN`, `POSTGRESQL_MAX_OPEN_CONNS`, `POSTGRESQL_MAX_IDLE_CONNS`, `POSTGRESQL_CONN_MAX_LIFETIME`, `POSTGRESQL_CONN_MAX_IDLE_TIME`.

### 4. Validation & Error Matrix

| Condition | Required behavior |
|---|---|
| Missing/invalid DSN or pool values | Fail before HTTP listen; never log the DSN |
| Ping fails | Close the newly created pool and return a wrapped error |
| Same content already active in one knowledge base | Return `domain.ErrDuplicate`; uploaded object is compensated if already written |
| Concurrent insert returns PostgreSQL `23505` | Map to `domain.ErrDuplicate`, not a generic service error |
| Second processing request cannot acquire row | Return already-running behavior; perform no slow work |
| Chunk/vector count differs | Reject before `BeginTxx` |
| Any vector length differs from 1536 | Reject before `BeginTxx`; keep old data |
| Insert/update fails in replacement | Roll back all deletes/inserts/status changes |
| Commit fails | Return commit error and attempt rollback without replacing the primary error |
| Document disappears during update | Return `domain.ErrNotFound` and roll back |

### 5. Good / Base / Bad Cases

- Good: a retry prepares a complete new result outside the transaction, replaces all prior chunks/vectors atomically, preserves metadata, and clears only `lastError`.
- Base: upload creates one `pending` document with `chunk_count = 0`; a later explicit processing request completes it.
- Bad: delete old vectors before Embedding finishes, write a 1024-dimensional vector, or run the baseline SQL against a populated database.

### 6. Tests Required

- Row-model round trip asserts every document field, JSON map, status, and timestamp survives conversion.
- Vector tests assert empty/wrong dimensions fail and valid values produce pgvector text.
- Repository tests assert `23505` mapping, conditional ownership behavior, pagination, logical document deletion, and SQL parameters.
- Transaction tests assert old-vector delete, old-chunk delete, inserts, completed update, commit order, and rollback on every injected failure.
- Config/database tests assert pool constraints, immediate Ping, Ping cleanup, and no DSN in returned errors.
- Integration tests, when enabled, use PostgreSQL with pgvector and never production credentials.

### 7. Wrong vs Correct

#### Wrong

```go
tx, _ := db.BeginTxx(ctx, nil)
_, _ = tx.ExecContext(ctx, "DELETE FROM rag.t_knowledge_chunk WHERE document_id=$1", id)
vectors, _ := embedder.EmbedTexts(ctx, texts) // slow external work while holding locks
_ = tx.Commit()
```

#### Correct

```go
vectors, err := embedder.EmbedTexts(ctx, texts)
if err != nil {
    return markProcessingFailed(ctx, document, err)
}
if err := validateVectors(vectors, 1536); err != nil {
    return markProcessingFailed(ctx, document, err)
}
return repository.ReplaceDocumentChunks(ctx, completed, chunks, vectors)
```
