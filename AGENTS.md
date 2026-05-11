# Repository Guidelines

## Project Structure & Module Organization

This repository contains a Spring Boot travel-agent service under `agent/`.
Application code lives in `agent/src/main/java/com/ken/agent`, with
`AgentApplication.java` as the startup class. Configuration files live in
`agent/src/main/resources`; prefer `application.yml` for Spring settings.
Tests belong under `agent/src/test/java/com/ken/agent`.

Keep new code grouped by responsibility, for example:

- `controller/` for REST endpoints
- `service/` for business and AI orchestration logic
- `dto/` for request and response records
- `repository/` or `mapper/` for database access if needed

## Build, Test, and Development Commands

Run commands from the `agent/` directory:

```bash
mvn spring-boot:run
```

Starts the local Spring Boot service.

```bash
mvn test
```

Runs the JUnit test suite.

```bash
mvn clean package
```

Builds the application jar under `agent/target/`.

The project currently targets Java 21, so configure IDEA and Maven Runner with
a JDK 21 installation.

## Coding Style & Naming Conventions

Use Java 21 and standard Spring Boot conventions. Indent Java and XML with four
spaces. Use `PascalCase` for classes, `camelCase` for methods and fields, and
clear suffixes such as `Controller`, `Service`, `Request`, and `Response`.

Prefer constructor injection. Keep controllers thin; place planning, RAG, and
database logic in services or infrastructure classes.

不要使用 @Autowired 字段注入。
使用构造器注入，配合 Lombok 的 @RequiredArgsConstructor。
参考示例：ChunkEmbeddingService.java 中的写法。

## Testing Guidelines

Tests use JUnit 5 via `spring-boot-starter-test`. Name test classes with the
`*Tests` suffix and place them in the matching package under `src/test/java`.

For Spring AI or PostgreSQL/pgvector behavior, prefer focused integration tests
with explicit test configuration rather than relying on production credentials.

## Commit & Pull Request Guidelines

The current history uses short initialization messages, including an `init:`
prefix. Continue with concise messages such as `feat: add travel plan endpoint`
or `fix: configure pgvector schema`.

Pull requests should describe the change, list local verification commands, and
mention any database, schema, or JVM parameter requirements.

## Security & Configuration Tips

Do not commit API keys, database passwords, `.env` files, or IDE workspace
metadata. Pass secrets through environment variables or JVM properties such as
`-DOPENAI_API_KEY=...` and `-DOPENAI_BASE_URL=...`.
