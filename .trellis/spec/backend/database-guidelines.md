# Database Guidelines

> Executable database contracts for the Spring Boot backend.

## Overview

- PostgreSQL is the only supported relational database.
- Ordinary business tables use MyBatis-Plus entities, mappers, and services.
- PostgreSQL-specific types that do not map safely through MyBatis-Plus, such as pgvector `vector`, use a narrow `JdbcTemplate` adapter behind a business service interface.
- The Go MVP is a parallel service under `go/`; it reuses the same `rag` schema and writes pgvector through explicit SQL casts, not through an ORM-mapped vector field.
- MyBatis-Plus and `JdbcTemplate` must use the same Spring-managed `DataSource` so one `TransactionOperations` boundary commits or rolls back both.
- Baseline and non-destructive upgrade SQL live in `resources/database/`.

## Scenario: Knowledge document ingestion and pgvector persistence

### 1. Scope / Trigger

- Trigger: adding or changing document upload, chunk replacement, embedding persistence, document processing status, or the `rag` database schema.
- This contract prevents startup failures from unmapped `PGvector` entity fields, duplicate documents under concurrent uploads, and partial replacement of chunks and vectors.

### 2. Signatures

- Upload API: `POST /api/knowledge/bases/{kb-id}/documents/upload` (`multipart/form-data`).
- Processing API: `POST /api/knowledge/documents/{doc-id}/chunk` with an optional JSON body.
- Status API: `GET /api/knowledge/documents/{doc-id}/status`.
- Vector boundary:

```java
void indexDocumentChunks(String kbId, String documentId, List<VectorChunk> chunks);
void upsertChunk(String kbId, String documentId, VectorChunk chunk);
void deleteDocumentVectors(String documentId);
void deleteChunkVector(String chunkId);
```

- Required vector schema and indexes:

```sql
embedding vector(1536) NOT NULL

CREATE INDEX idx_kv_embedding
ON rag.t_knowledge_vector
USING hnsw (embedding vector_cosine_ops);

CREATE UNIQUE INDEX uk_knowledge_document_kb_hash_active
ON rag.t_knowledge_document (kb_id, content_hash)
WHERE deleted = 0 AND content_hash IS NOT NULL;
```

### 3. Contracts

- Upload and processing are separate explicit calls. A successful upload creates a `pending` document with `chunk_count = 0`; it must not invoke parsing or embedding.
- Duplicate identity is `kb_id + SHA-256(file bytes)` for active rows. File names do not determine duplication.
- Processing status values are `pending`, `processing`, `completed`, and `failed`.
- Only a conditional database update may acquire processing ownership: update to `processing` where the current state is not already `processing`; zero updated rows means the request did not acquire ownership.
- Parsing, chunking, and embedding run before the replacement transaction. Only complete, validated new results enter the transaction.
- The replacement transaction physically removes old chunks, deletes old vectors, inserts new chunks, batch-inserts new vectors, and updates the document to `completed`.
- Failure metadata is merged into the existing document metadata under `lastError`; success removes only `lastError`.
- Vector SQL binds metadata as JSON text and embeddings as pgvector text:

```sql
INSERT INTO rag.t_knowledge_vector (id, content, metadata, embedding)
VALUES (?, ?, ?::jsonb, ?::vector)
ON CONFLICT (id) DO UPDATE SET
  content = EXCLUDED.content,
  metadata = EXCLUDED.metadata,
  embedding = EXCLUDED.embedding;
```

- Environment/config keys:
  - `KNOWLEDGE_DOCUMENT_ALLOWED_EXTENSIONS`, default `pdf,doc,docx,txt,md,markdown,html,htm`.
  - `KNOWLEDGE_DOCUMENT_MAX_SIZE`, default `50MB`.
  - `spring.ai.openai.embedding.options.dimensions`, currently `1536`; changing it requires the database column, validation constant, and tests to change together.

### 4. Validation & Error Matrix

| Condition | Required behavior |
|---|---|
| Empty file | Reject before object storage with a client error |
| Extension not allowed | Reject before object storage with a client error naming the extension |
| File exceeds configured maximum | Reject before reading all bytes or uploading |
| Same content in the same knowledge base | Reject before upload when pre-check finds it |
| Concurrent duplicate reaches the unique index | Convert `DuplicateKeyException` to the duplicate client error and compensate the uploaded object |
| Object uploaded but document insert fails | Best-effort delete the uploaded object; rethrow the original database error |
| Second processing request | Conditional update returns zero; reject without parsing the file |
| Parsed text/chunk list/embedding is empty | Mark processing failed; do not enter the replacement transaction |
| Embedding dimension is not 1536 | Mark processing failed; keep the previous chunks and vectors |
| Replacement database operation fails | Roll back old-chunk deletion, old-vector deletion, and all new writes together |
| Retry succeeds | Set `completed`, replace all old results, preserve other metadata, remove `lastError` |

### 5. Good / Base / Bad Cases

- Good: two files with the same name but different bytes in one knowledge base are accepted; after explicit processing, chunk and vector IDs match and the document is `completed`.
- Base: one valid upload returns `pending`; a later explicit chunk request synchronously produces non-empty 1536-dimensional vectors.
- Bad: two concurrent uploads of identical bytes create two objects and two active rows, or a retry deletes old vectors before new embeddings have been fully validated.

### 6. Tests Required

- Unit: configured extension and size binding; empty/disallowed/oversized upload rejection before storage access.
- Unit: SHA-256 duplicate pre-check, database-insert compensation, unique-conflict conversion, and compensation failure preserving the original exception.
- Unit: conditional processing rejection, empty parse/chunk/vector rejection before transaction, failure metadata merge, and successful retry clearing only `lastError`.
- Unit: `JdbcTemplate.batchUpdate` is called once for multiple chunks and binds `kbId`, `documentId`, `chunkId`, `chunkIndex`, JSON, and vector text.
- Context: Spring starts without `No typehandler found for property embedding` and without calling real model APIs.
- Integration with `pgvector/pgvector:pg16`: baseline SQL executes on an empty database, 1536-dimensional vectors are written, duplicate scope is enforced, and a forced transaction failure restores both the old chunk and old vector.
- Build gates: root `mvn test`, root `mvn clean package`, and `git diff --check`.

### 7. Wrong vs Correct

#### Wrong

```java
// A BaseMapper scans the PGvector field at startup, but no TypeHandler exists.
interface KnowledgeVectorMapper extends BaseMapper<KnowledgeVectorEntity> {
}

// Old data is removed before slow parsing and embedding have succeeded.
knowledgeVectorService.deleteDocumentVectors(documentId);
List<VectorChunk> chunks = embeddingService.embed(parse(file));
```

#### Correct

```java
// Slow and failure-prone preparation happens first and outside the DB transaction.
List<VectorChunk> chunks = extractChunkEmbedAndValidate(document);

transactionOperations.executeWithoutResult(status -> {
    chunkService.deletePhysicallyByDocumentId(document.getId());
    vectorService.deleteDocumentVectors(document.getId());
    chunkService.saveBatch(toEntities(chunks));
    vectorService.indexDocumentChunks(document.getKbId(), document.getId(), chunks);
    markCompletedAndClearLastError(document, chunks.size());
});
```

The correct form keeps ordinary tables on MyBatis-Plus, isolates pgvector SQL behind `KnowledgeVectorService`, and makes both participate in one Spring transaction.

## Scenario: Go TravelAgent MVP parallel service

### 1. Scope / Trigger

- Trigger: adding or changing the Go MVP under `go/`, especially knowledge document upload, explicit chunking, embedding generation, pgvector writes, S3/RustFS storage, or README/runtime env documentation.
- This contract keeps the Go service compatible with the existing Java service and prevents schema drift between the two implementations.

### 2. Signatures

- Go module root: `go/`.
- Go service entrypoint: `go/cmd/travel-agent/main.go`.
- Default Go port: `8081`.
- Go API paths reuse the Java knowledge paths:

```text
POST   /api/knowledge/bases/{kb-id}/documents/upload
POST   /api/knowledge/documents/{doc-id}/chunk
GET    /api/knowledge/documents/{doc-id}
GET    /api/knowledge/documents/{doc-id}/status
GET    /api/knowledge/bases/{kb-id}/documents
DELETE /api/knowledge/documents/{doc-id}
```

- Go repository boundary:

```go
KnowledgeBaseExists(ctx context.Context, kbID string) (bool, error)
ActiveDocumentHashExists(ctx context.Context, kbID string, contentHash string) (bool, error)
CreateDocument(ctx context.Context, doc Document) error
TryMarkProcessing(ctx context.Context, docID string) (Document, bool, error)
ReplaceDocumentChunks(ctx context.Context, doc Document, chunks []Chunk, vectors [][]float32) error
MarkFailed(ctx context.Context, doc Document, message string) error
```

### 3. Contracts

- Java remains in `framework/` and `bootstrap/`; the Go MVP must not rename or delete Java modules.
- Go code lives under `go/` and uses:
  - Gin for HTTP.
  - `sqlx + pgx` for PostgreSQL.
  - AWS SDK for Go v2 for S3/RustFS-compatible storage.
- Go responses should match the Java `Result<T>` shape: `code`, `message`, `data`; success code is `"0"`.
- Go writes embeddings as pgvector text with an explicit cast:

```sql
INSERT INTO rag.t_knowledge_vector (id, content, metadata, embedding)
VALUES ($1, $2, $3::jsonb, $4::vector)
```

- Go test commands must set local cache paths when the default Windows user or Maven/Go cache directories are not writable:

```powershell
$env:GOTELEMETRY='off'
$env:GOTELEMETRYDIR='<repo>\go\.cache\telemetry'
$env:GOCACHE='<repo>\go\.cache\build'
$env:GOMODCACHE='<repo>\.trellis\workspace\go-mod-cache'
go test ./...
```

- Local cache directories must stay ignored:
  - `go/.cache/`
  - `.trellis/workspace/go-build-cache/`
  - `.trellis/workspace/go-mod-cache/`
  - `.trellis/workspace/go-telemetry/`

### 4. Validation & Error Matrix

| Condition | Required behavior |
|---|---|
| Missing `POSTGRESQL_DSN` | Go startup fails clearly before serving traffic |
| Empty file | Reject before object storage |
| File exceeds `KNOWLEDGE_DOCUMENT_MAX_SIZE` | Reject before object storage |
| Disallowed extension | Reject before object storage |
| Same content in the same knowledge base | Reject before upload when pre-check catches it |
| Unique index catches concurrent duplicate | Convert PostgreSQL `23505` to the duplicate business error and compensate uploaded object |
| Object stored but document create fails | Best-effort delete stored object and return original error |
| Second chunk request while processing | Conditional update fails to acquire ownership; reject without parsing |
| Unsupported Go parser format (`pdf`, `doc`, `docx` in MVP) | Mark document `failed` with `metadata.lastError` |
| Embedding count/dimension mismatch | Mark document `failed`; do not replace old chunks/vectors |
| Replacement transaction fails | Roll back chunk/vector replacement and document completion update together |

### 5. Good / Base / Bad Cases

- Good: Go uploads a `.txt` file to a valid knowledge base, returns `pending`, then explicit chunking writes chunks and 1536-dimensional vectors and returns `completed`.
- Base: Go service runs on `8081` with the same `/api/knowledge/...` paths while Java keeps running separately on `8080`.
- Bad: Go creates its own schema/table names, changes vector dimensions, or introduces a different response envelope that callers cannot swap with Java.

### 6. Tests Required

- Unit: config defaults and env overrides for port, upload limits, allowed extensions, embedding dimensions.
- Unit: pgvector text formatting rejects empty vectors and validates exact dimensions.
- Unit: chunking keeps order, positions, and source substrings.
- Unit: upload rejects duplicates before storage and compensates storage after document-create failure.
- Unit: processing success clears only `lastError` and preserves other metadata.
- Unit: processing failure records latest error and does not replace chunks/vectors.
- Build gates: `go test ./...`, `go build ./cmd/travel-agent`, root `mvn test`, root `mvn clean package`, and `git diff --check`.

### 7. Wrong vs Correct

#### Wrong

```go
// Different path and different response shape make Java/Go callers diverge.
router.POST("/api/go/documents/upload", upload)
c.JSON(200, gin.H{"ok": true, "payload": doc})

// Slow embedding runs inside the DB transaction and can leave locks open.
tx := db.MustBegin()
vectors := embeddingClient.EmbedTexts(ctx, texts)
insertVectors(tx, vectors)
tx.Commit()
```

#### Correct

```go
// Same API path and Java-compatible Result shape.
api.POST("/api/knowledge/bases/:kbID/documents/upload", handler.upload)
c.JSON(http.StatusOK, httpapi.Success(doc))

// Slow work happens first. Only complete, validated results enter the transaction.
vectors, err := embedder.EmbedTexts(ctx, texts)
if err != nil {
    repo.MarkFailed(ctx, doc, err.Error())
    return err
}
return repo.ReplaceDocumentChunks(ctx, doc, chunks, vectors)
```

## Common Mistakes

- Do not add a `PGvector` field to a MyBatis-Plus entity unless a tested TypeHandler is explicitly configured for every read/write path.
- Do not use a service-layer duplicate query as the only protection; concurrent uploads require the partial unique index.
- Do not use logical deletion when atomically replacing all chunks for a document; stale rows remain and can conflict with the new result.
- Do not overwrite the whole metadata map when recording `lastError`.
- Do not make migration scripts silently delete historical duplicates. Detect them and stop for manual resolution.
- Do not commit Go local caches or build binaries; keep `go/.cache/`, Go build outputs, and `.trellis/workspace/go-*` caches ignored.
