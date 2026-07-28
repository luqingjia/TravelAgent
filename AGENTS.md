# Repository Guidelines

## Project Structure

TravelAgent is a frontend/backend monorepo with an independent root-level Docker deployment directory. Run language-specific commands from the corresponding module directory.

- `backend/cmd/travel-agent/`: process entry point; only handles signals, calls `app.Run`, and selects the exit code.
- `backend/internal/app/`: the only backend composition root; creates concrete dependencies and manages the HTTP/database lifecycle.
- `backend/internal/knowledge/domain/`: knowledge-document aggregate, state transitions, chunk value objects, and domain errors; imports only the standard library.
- `backend/internal/knowledge/application/`: upload, process, query, and delete use cases, plus consumer-owned repository, storage, and Embedding interfaces.
- `backend/internal/knowledge/adapter/http/`: Gin routes, request/response DTOs, and error mapping.
- `backend/internal/knowledge/adapter/postgres/`: sqlx row models, SQL, pgvector conversion, and replacement transactions.
- `backend/internal/platform/`: configuration, database connection, HTTP middleware, object storage, and Embedding client.
- `backend/migrations/`: database SQL for manual review and execution; the application must never run it automatically.
- `frontend/`: Vue 3, TypeScript, and Vite frontend.
- `docker/`: repository-level Dockerfile, Compose configuration, environment template, and initialization seed.

Do not create empty future modules before real business behavior exists. Do not add catch-all packages such as `common`, `utils`, or `models`.

## Dependency Boundaries

- `domain` may import only the Go standard library.
- `application` may import `domain`, but must not import Gin, sqlx, pgx, the AWS SDK, or concrete platform implementations.
- HTTP adapters must not directly access concrete database, object-storage, or Embedding implementations.
- PostgreSQL, storage, and Embedding adapters implement the small interfaces defined by `application`.
- Only `backend/internal/app` may import concrete adapters together and assemble the full backend object graph.
- Inject dependencies manually through constructors. Do not introduce a DI container, and do not store databases or services in globals.
- `gin.Context` is only for request-scoped data such as request IDs, authenticated subjects, and traces. It must not be used as a service locator.

## Build, Test, and Run

Run backend commands from `backend/`:

```powershell
cd backend
go test ./...
go vet ./...
go build -o ../.trellis/workspace/bin/travel-agent.exe ./cmd/travel-agent
go run ./cmd/travel-agent
```

Run frontend commands from `frontend/`, for example `pnpm install`, `pnpm dev`, `pnpm lint`, `pnpm test:unit`, and `pnpm build`. Run Docker Compose, Git, Trellis, and documentation checks from the repository root.

`backend/.env.example` is only a template. The program does not automatically load `.env`. For local runs, inject environment variables explicitly through PowerShell, the IDE, or a container.

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
- Before completion, from `backend/` at minimum pass `go fmt ./...`, `go test ./...`, `go vet ./...`, and `go build -o ../.trellis/workspace/bin/travel-agent.exe ./cmd/travel-agent`; then run `git diff --check` from the repository root.

## Tool-assisted Discovery

This repository expects agents to use the available MCP/tool stack before broad `rg`/`find` loops when those tools can answer the question.

### Preferred tools

| Tool | Use when | Do not use for |
|---|---|---|
| **codegraph** (`codegraph_explore`) | “How does X work?”, architecture, call paths, blast radius, or preparing an edit in an indexed tree | Non-code docs, secrets, generated caches |
| **codebase-memory** graph tools | Symbol lookup, callers/callees, impact analysis, cross-file relationships, architecture overview | Full-text prose search in Markdown/config when the graph has no nodes |
| **Context7** (`resolve-library-id` → `query-docs`) | Current third-party library/API docs (Gin, sqlx, pgx, AWS SDK, Eino, Vue, Vite, UI libs, etc.) | Project-local business rules already written in `.trellis/spec/` |
| `rg` / `find` / direct `read` | Graph/tools miss, non-code text, lockfiles, SQL, env templates, comments, or no index exists | First-choice replacement for symbol/architecture questions when tools are available |

### Operating rules

1. **Library/API questions**: call Context7 first for the relevant version; do not invent APIs from memory.
2. **Local symbols / call graph / impact**: prefer codegraph explore or codebase-memory search/trace over recursive grepping.
3. **Fallback**: if MCP tools are unavailable, unindexed, or return empty/stale results, fall back to `rg` + `read` and say so briefly when it affects confidence.
4. **Indexes are optional local artifacts**: `.codegraph/` is gitignored; codebase-memory indexes are local. Do not commit index databases or treat them as source of truth over the files.
5. **Do not auto-reindex large trees** unless the user asks or the index is clearly missing/broken for the area you must change.
6. **Specs still win for contracts**: `.trellis/spec/` and task `prd.md`/`design.md` override generic library advice when they conflict on project boundaries.

## Security and Configuration

- Do not commit API keys, database passwords, object-storage secrets, `.env`, local data, caches, or build artifacts.
- Logs must not output DSNs, Authorization headers, API keys, access keys, secret keys, or full uploaded content.
- `backend/migrations/000001_rag_baseline.sql` is only for a brand-new empty database. All SQL must be reviewed manually, and application startup must never execute migrations automatically.
- `.dockerignore` stays at the repository root because Docker builds with the repository root as its context; the Dockerfile must copy only the required `backend/` files.

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
