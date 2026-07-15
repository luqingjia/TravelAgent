# CLAUDE.md

This file describes the current constraints that must be followed when developing in the TravelAgent repository. For the complete execution rules, see `AGENTS.md` and `.trellis/spec/backend/`.

## Project Overview

TravelAgent is a Go 1.26 single-service project. It currently supports travel knowledge-document upload, content deduplication, explicit chunking, Embedding, and PostgreSQL/pgvector persistence. The service listens on `8081` by default. Object storage supports both S3/RustFS and local-directory modes.

## Common Commands

Run all commands from the repository root:

```powershell
go test ./...
go vet ./...
go build -o .trellis/workspace/bin/travel-agent.exe ./cmd/travel-agent
go run ./cmd/travel-agent
```

`.env.example` is not loaded automatically. Before running the service, inject `POSTGRESQL_DSN`, `EMBEDDING_API_KEY`, and the required object-storage settings through the terminal, IDE, or deployment environment.

## Architecture Boundaries

```text
cmd -> internal/app -> concrete adapter/platform
                         |
HTTP adapter -> application <- PostgreSQL/Storage/Embedding
                      |
                    domain
```

- `internal/knowledge/domain` imports only the standard library and owns document state rules.
- `internal/knowledge/application` owns use cases and small interfaces for external capabilities.
- `internal/knowledge/adapter` handles frameworks, protocols, SQL, and model conversion.
- `internal/platform` provides reusable process-level infrastructure.
- `internal/app` is the only composition root. It uses constructor-based manual injection, not service location or global databases.

## Data and Runtime Contracts

- Preserve the `rag` schema and `vector(1536)`.
- Preserve the six `/api/knowledge/...` routes and the `code/message/data` response envelope.
- A successful upload only creates a `pending` document; chunking must be triggered explicitly.
- Parsing, chunking, and Embedding run outside the transaction. Only a complete result enters the replacement transaction.
- If database creation fails after an object upload, compensate the uploaded object on a best-effort basis. Compensation errors must not replace the original error.
- Request logs use `slog`, include `request_id/method/path/status/latency_ms`, and must not record secrets or DSNs.
- After receiving `SIGINT/SIGTERM`, perform graceful shutdown within the configured timeout.

## Coding and Testing

- Behavior changes follow the red-test, minimal implementation, green-test, refactor sequence.
- Wrap errors with `%w`, classify them with `errors.Is/As`, and pass `context.Context` as the first argument down the call chain.
- Production code uses accurate, detailed, plain Chinese comments. Tests explain scenarios, setup, and key assertions.
- Before completion, run `go fmt ./...`, `go test ./...`, `go vet ./...`, build, and `git diff --check`.

## Important Files

- `README.md`: runtime, configuration, API, and MVP boundaries.
- `.env.example`: complete environment-variable template without real credentials.
- `migrations/000001_rag_baseline.sql`: only for a brand-new empty database.
- `migrations/000002_knowledge_ingestion_upgrade.sql`: non-destructive check/upgrade script for an existing schema.
- `.trellis/tasks/07-13-go-enterprise-structure-comments/`: current refactor requirements, design, and implementation checklist.
