# Backend Development Guidelines

> Executable contracts for the TravelAgent Go service.

## Overview

TravelAgent is a frontend/backend monorepo. The Go service is a self-contained module rooted at `backend/`; bounded contexts currently include `knowledge` (document ingestion/RAG) and `agent` (Eino ChatModelAgent, model catalog, JSON/SSE chat). Both use a lightweight DDD and ports-and-adapters structure. Docker deployment assets are repository-level under `docker/`. These documents describe rules that must be visible in backend code, tests, and validation commands.

## Guidelines Index

| Guide | Contract | Status |
|---|---|---|
| [Directory Structure](./directory-structure.md) | Package ownership, dependency direction, composition root | Active |
| [Database Guidelines](./database-guidelines.md) | PostgreSQL/pgvector models, transaction order, migrations | Active |
| [Error Handling](./error-handling.md) | Domain errors, `%w`, HTTP mapping, cleanup errors | Active |
| [Logging Guidelines](./logging-guidelines.md) | `slog`, access fields, secret redaction | Active |
| [Quality Guidelines](./quality-guidelines.md) | TDD, Chinese comments, backend-module quality gates | Active |

## Pre-Development Checklist

1. Read [Directory Structure](./directory-structure.md) before adding or moving any package.
2. Read [Database Guidelines](./database-guidelines.md) before changing repository SQL, vector dimensions, document state persistence, or files in `backend/migrations/`.
3. Read [Error Handling](./error-handling.md) before adding an error path or HTTP response mapping.
4. Read [Logging Guidelines](./logging-guidelines.md) before adding middleware, external clients, configuration, or logs.
5. Read [Quality Guidelines](./quality-guidelines.md) before implementation and before reporting completion.
6. For changes spanning three or more layers, also read `../guides/cross-layer-thinking-guide.md`.
7. For symbol lookup, call-path impact, or third-party library APIs, follow `../guides/tool-assisted-discovery.md` (codegraph, codebase-memory, Context7 before broad grep).

All backend code-spec documents are written in English. Production and test code comments are written in detailed, plain Chinese as required by the repository.
