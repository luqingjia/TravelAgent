# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

TravelAgent is a Spring Boot 3.5 application (Java 21) that implements a RAG-based travel planning assistant. It manages knowledge bases, parses and chunks travel documents, generates embeddings via OpenAI-compatible APIs, stores vectors in PostgreSQL/pgvector, and retrieves relevant context to answer user travel questions.

## Build & Development Commands

This is a Maven multi-module project with root POM and two modules: `framework` (shared utilities) and `bootstrap` (main application).

Build the entire project:
```bash
mvn clean package
```

Run the application (from project root or `bootstrap/`):
```bash
mvn spring-boot:run -pl bootstrap
```

Run all tests:
```bash
mvn test
```

Run a single test class:
```bash
mvn test -pl bootstrap -Dtest=AgentApplicationTests
```

Run a single test method:
```bash
mvn test -pl bootstrap -Dtest=AgentApplicationTests#embeddingModelTest
```

## Architecture

### Module Structure

- **`framework/`** — Shared infrastructure: exception hierarchy (`ClientException`, `ServiceException`, `AbstractException`), error code enums (`BaseErrorCode` following Alibaba error code conventions: A=client, B=service, C=remote), and database configuration. Has no Spring Boot web dependencies; intended as a reusable library module.
- **`bootstrap/`** — Main Spring Boot application (`AgentApplication`). Contains all business logic, REST controllers, services, MyBatis mappers, and document/AI processing pipelines.

### RAG Pipeline Architecture

The core RAG flow is being built incrementally:

```
Document Upload → Parse (Tika/Markdown) → Chunk (FixedSize/StructureAware)
  → Embed (Spring AI EmbeddingModel) → Store (PgVector)
  → Retrieve (similarity search) → Chat (OpenAI-compatible ChatModel)
```

Key architectural components (all in `bootstrap`):

- **`core.parser`** — `DocumentParser` interface with `TikaDocumentParser` and `MarkdownDocumentParser` implementations. Extracts plain text from uploaded files.
- **`core.chunk`** — `ChunkingStrategy` interface with `FixedSizeTextChunker` and `StructureAwareTextChunker`. `ChunkingStrategyFactory` auto-discovers all strategies via Spring's `List<ChunkingStrategy>` injection and maps them by `ChunkingEnum`. `ChunkEmbeddingService` batches text through `EmbeddingModel` and populates `VectorChunk` objects with float arrays.
- **`knowledge/`** — Domain layer for knowledge base management: `KnowledgeBase`, `KnowledgeDocument`, `KnowledgeChunk` entities with associated controllers, services, and MyBatis Plus mappers. Currently has placeholder implementations being filled in.
- **`model/`** — Enums for AI model capabilities and providers.

### Database & Vector Storage

- PostgreSQL with `pgvector` extension (`vector` type, `vector_cosine_ops` index).
- Schema `rag` contains four main tables: `t_knowledge_base`, `t_knowledge_document`, `t_knowledge_chunk`, `t_knowledge_vector`.
- MyBatis Plus is used for ORM; mappers extend `BaseMapper<DO>`.
- SQL schema and indexes are in `resources/database/rag.sql`.

### AI Model Configuration

Configured in `bootstrap/src/main/resources/application.yml`:

- **Chat model**: Silicon Flow API (`https://api.siliconflow.cn`) using `Qwen/Qwen3.6-35B-A3B`. Key: `Silicon_Flow_API_KEY`.
- **Embedding model**: Alibaba BaiLian (`https://dashscope.aliyuncs.com/compatible-mode`) using `text-embedding-v3` with 1536 dimensions. Key: `BaiLian_API_KEY`.
- **Database**: `jdbc:postgresql://localhost:5432/kenagent`. Credentials via `POSTGRESQL_USER` and `POSTGRESQL_PASSWORD` env vars.

Virtual threads are enabled (`spring.threads.virtual.enabled: true`).

## Coding Conventions

- **Java 21**, four-space indentation.
- **Constructor injection only** — never use `@Autowired` field injection. Use Lombok's `@RequiredArgsConstructor`.
- Follow existing error code convention (`BaseErrorCode` enum: A=client, B=service, C=remote) or extend `IErrorCode` for domain-specific codes.
- Controllers are thin; business logic belongs in services or infrastructure classes.
- Test classes use `*Tests` suffix under `src/test/java` in the matching package.

## Important Files

- `doc/knowledge-implementation-plan.md` — Detailed phased implementation plan for the RAG knowledge system. Good reference for intended architecture and recommended implementation order.
- `resources/database/rag.sql` — PostgreSQL schema with pgvector tables and indexes.
- `AGENTS.md` — Additional repository guidelines (keep in sync when editing conventions).

## Coding Style
- 不要使用 **@Autowired** 字段注入。
- 使用构造器注入，配合 **Lombok** 的 **@RequiredArgsConstructor**。
- 参考示例：**ChunkEmbeddingService.java** 中的写法。