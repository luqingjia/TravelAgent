# Directory Structure

## Scenario: Add or change a backend capability

### 1. Scope / Trigger

- Trigger: adding a use case, adapter, infrastructure client, executable, or moving Go files between packages.
- Goal: keep business rules independent from Gin, SQL, storage SDKs, environment variables, and process lifecycle code.

### 2. Signatures

Required backend-module layout:

```text
backend/
├── cmd/travel-agent
├── internal/app
├── internal/platform/{config,database,embedding,httpserver,storage}
├── internal/knowledge/{domain,application,adapter/http,adapter/postgres}
├── internal/agent/{domain,application,adapter/eino,adapter/http}
└── migrations
```

Constructor pattern:

```go
func NewService(deps application.Dependencies) (*application.Service, error)
func NewHandler(service httpadapter.KnowledgeService, logger *slog.Logger) (*httpadapter.Handler, error)
func New(ctx context.Context, cfg config.Config) (*app.App, error)
```

### 3. Contracts

- `backend/internal/knowledge/domain` and `backend/internal/agent/domain` import only the Go standard library and own invariants for their bounded contexts.
- `application` packages import their own `domain`, own use cases, and define the smallest ports they consume.
- `knowledge` and `agent` must not import each other.
- `adapter/http` packages import Gin plus inward packages, but never concrete database/storage/embedding/Eino implementations except through composition root wiring.
- `agent/adapter/eino` implements agent application runtime/catalog ports and may import Eino/OpenAI adapters.
- `adapter/postgres` implements knowledge application repository interfaces and owns row models, SQL, JSON/pgvector casts, and transactions.
- `platform` owns reusable process infrastructure (including root Gin router/health); it must not own knowledge or agent business rules.
- `app` is the only backend composition root allowed to import all concrete adapters; repository-level `frontend/`, `docker/`, and tooling remain outside `backend/`.
- `cmd` handles operating-system signals, calls `app.Run`, logs a final error, and selects the exit code.
- Do not create empty future modules or catch-all packages named `common`, `utils`, or `models`.

### 4. Validation & Error Matrix

| Condition | Required behavior |
|---|---|
| Domain imports a third-party module | Reject in review; move protocol/framework code outward |
| Application imports Gin/sqlx/AWS SDK | Define or narrow an application-owned interface |
| Handler needs a database/client | Inject an application use-case interface instead |
| Constructor dependency is nil/invalid | Return a specific construction error before serving traffic |
| Construction fails after DB open | Close the database once and preserve both primary/cleanup errors |
| New business area has no behavior yet | Do not create a placeholder package |

### 5. Good / Base / Bad Cases

- Good: a new storage implementation satisfies `application.ObjectStorage`, and only `internal/app` selects it.
- Good: agent HTTP and Eino adapters implement application-owned ports; only `internal/app` constructs Runtime and registers `/api/agent`.
- Base: a new knowledge or agent use case is implemented in its `application` package, with rules in `domain` and transport conversion in an adapter.
- Bad: a Gin handler creates `sqlx.DB`, reads environment variables, or stores a service singleton in `gin.Context`.
- Bad: knowledge and agent packages import each other, or application imports Eino/Gin types.

### 6. Tests Required

- Constructor tests assert missing dependencies fail before external work.
- Composition-root tests assert exact creation order (including agent after knowledge handler) and database cleanup when agent runtime construction fails after DB open.
- Package tests use fakes at application-owned interfaces instead of starting all infrastructure.
- Final architecture validation checks imports: domain standard-library-only; application has no Gin/sqlx/pgx/AWS SDK; only app imports concrete adapters together.

### 7. Wrong vs Correct

#### Wrong

```go
func upload(c *gin.Context) {
    db := globalDB
    service := c.MustGet("service").(*application.Service)
    _ = db
    _ = service
}
```

#### Correct

```go
func NewHandler(service KnowledgeService, logger *slog.Logger) (*Handler, error) {
    if service == nil || logger == nil {
        return nil, errors.New("handler dependencies are required")
    }
    return &Handler{service: service, logger: logger}, nil
}
```
