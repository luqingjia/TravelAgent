# Repository Guidelines

## Project Structure

TravelAgent is a Go single-service project that is built and run from the repository root.

- `cmd/travel-agent/`: process entry point; only handles signals, calls `app.Run`, and selects the exit code.
- `internal/app/`: the only composition root; creates concrete dependencies and manages the HTTP/database lifecycle.
- `internal/knowledge/domain/`: knowledge-document aggregate, state transitions, chunk value objects, and domain errors; imports only the standard library.
- `internal/knowledge/application/`: upload, process, query, and delete use cases, plus small repository, storage, and Embedding interfaces defined by the consumer.
- `internal/knowledge/adapter/http/`: Gin routes, request/response DTOs, and error mapping.
- `internal/knowledge/adapter/postgres/`: sqlx row models, SQL, pgvector conversion, and replacement transactions.
- `internal/platform/`: configuration, database connection, HTTP middleware, object storage, and Embedding client.
- `migrations/`: database SQL for manual review and execution; the application must never run it automatically.

Do not create empty future modules before real business behavior exists. Do not add catch-all packages such as `common`, `utils`, or `models`.

## Dependency Boundaries

- `domain` may import only the Go standard library.
- `application` may import `domain`, but must not import Gin, sqlx, pgx, the AWS SDK, or concrete platform implementations.
- HTTP adapters must not directly access concrete database, object-storage, or Embedding implementations.
- PostgreSQL, storage, and Embedding adapters implement the small interfaces defined by `application`.
- Only `internal/app` may import concrete adapters together and assemble the full object graph.
- Inject dependencies manually through constructors. Do not introduce a DI container, and do not store databases or services in globals.
- `gin.Context` is only for request-scoped data such as request IDs, authenticated subjects, and traces. It must not be used as a service locator.

## Build, Test, and Run

Run all commands from the repository root:

```powershell
go test ./...
go vet ./...
go build -o .trellis/workspace/bin/travel-agent.exe ./cmd/travel-agent
go run ./cmd/travel-agent
```

`.env.example` is only a template. The program does not automatically load `.env`. For local runs, inject environment variables explicitly through PowerShell, the IDE, or a container.

## Go Conventions

- Use `gofmt`. Package names should be short and lowercase. Exported identifiers use `PascalCase`; internal identifiers use `camelCase`.
- Functions that accept `context.Context` put it first and pass the caller's context down to database, HTTP, and storage operations.
- Wrap errors with operation context using `fmt.Errorf("operation: %w", err)`. Classify errors with `errors.Is/As`; do not compare error strings.
- Constructors validate long-lived dependencies and stable configuration. Return errors immediately when something is missing; do not defer construction failures to the first request.
- External DTOs, database row models, and domain objects must remain separate and must be converted explicitly at adapter boundaries.
- pgvector is fixed at 1536 dimensions. SQL writes must use explicit `::vector` casts; do not rely on the driver to infer PostgreSQL-specific types.
- Slow document chunking work runs outside transactions. Replacing old chunks, old vectors, new data, and the completed status must happen in one short transaction.

## Commenting Requirements

- Every production package needs a package comment that explains its responsibility and what it must not do.
- Production code must include accurate, plain Chinese comments for structs, interfaces, functions, key steps, and non-obvious statements.
- Business use cases should explain validation, data changes, state transitions, external calls, transaction boundaries, and failure compensation in real execution order.
- Test code should explain the scenario, setup, failure injection, and key assertions. Do not mechanically translate boilerplate with no business meaning.
- Comments should explain why the code exists and what happens on failure. Do not write syntax narration such as "assign this variable" or "enter the if".

## Testing Guidelines

- For behavior changes, write the failing test first, confirm the failure reason, then implement the smallest fix and run regression checks.
- Domain tests cover state transitions and invariants. Application tests use fake ports to cover orchestration and compensation. Adapter tests cover boundary conversion, SQL/vector formatting, and HTTP compatibility.
- Tests must not depend on real cloud credentials, fixed development ports, or production databases.
- Before completion, at minimum pass `go fmt ./...`, `go test ./...`, `go vet ./...`, `go build ./cmd/travel-agent`, and `git diff --check`.

## Tool-assisted Discovery

- When checking current APIs for libraries such as Gin, sqlx, pgx, or the AWS SDK, use Context7 first to fetch official documentation for the relevant version.
- When looking up code symbols, call relationships, or dependency impact, prefer the codebase-memory graph. Use `rg` only when the graph is insufficient or when searching non-code text.

## Security and Configuration

- Do not commit API keys, database passwords, object-storage secrets, `.env`, local data, caches, or build artifacts.
- Logs must not output DSNs, Authorization headers, API keys, access keys, secret keys, or full uploaded content.
- `migrations/000001_rag_baseline.sql` is only for a brand-new empty database. All SQL must be reviewed manually, and application startup must never execute migrations automatically.

## Commit and Review

Keep commit messages short, for example `refactor(go): standardize DDD project structure`. Commit or PR descriptions should list verification commands and call out database, pgvector, environment-variable, or migration requirements.

<!-- TRELLIS:START -->
# Trellis Instructions

These instructions are for AI assistants working in this project.

This project is managed by Trellis. The working knowledge you need lives under `.trellis/`:

- `.trellis/workflow.md` — development phases, when to create tasks, skill routing
- `.trellis/spec/` — package- and layer-scoped coding guidelines (read before writing code in a given layer)
- `.trellis/workspace/` — per-developer journals and session traces
- `.trellis/tasks/` — active and archived tasks (PRDs, research, jsonl context)

If a Trellis command is available on your platform (e.g. `/trellis:finish-work`, `/trellis:continue`), prefer it over manual steps. Not every platform exposes every command.

If you're using Codex or another agent-capable tool, additional project-scoped helpers may live in:
- `.agents/skills/` — reusable Trellis skills
- `.codex/agents/` — optional custom subagents

Managed by Trellis. Edits outside this block are preserved; edits inside may be overwritten by a future `trellis update`.

<!-- TRELLIS:END -->
