# Error Handling

## Scenario: Propagate a failure across domain, application, adapter, and process boundaries

### 1. Scope / Trigger

- Trigger: adding validation, external calls, cleanup/compensation, repository behavior, constructors, or HTTP endpoints.
- Goal: preserve stable business classification while retaining technical context for server-side diagnosis.

### 2. Signatures

Stable domain categories:

```go
var (
    ErrNotFound         = errors.New("not found")
    ErrDuplicate        = errors.New("same content already exists")
    ErrAlreadyRunning   = errors.New("document is processing")
    ErrInvalidArgument  = errors.New("invalid argument")
    ErrInvalidTransition = errors.New("invalid document status transition")
)
```

HTTP envelope and codes:

```go
type Result struct {
    Code    string `json:"code"`
    Message string `json:"message"`
    Data    any    `json:"data"`
}

const SuccessCode = "0"
const ClientErrorCode = "A000001"
const ServiceErrorCode = "B000001"
```

### 3. Contracts

- Add operation context with `fmt.Errorf("operation: %w", err)`; never classify by string matching.
- Use `errors.Is` for sentinel categories and `errors.As` for typed driver/protocol errors.
- Domain creates domain errors and legal state transitions. Repositories and external clients wrap failures but do not invent duplicate domain rules.
- `ErrNotFound` maps to HTTP 404 + `A000001`.
- Invalid argument, duplicate, already-running, and invalid-transition errors map to HTTP 400 + `A000001`.
- Unknown infrastructure failures map to HTTP 500 + `B000001`; the client receives a generic message, while the full wrapped error is logged server-side.
- Result JSON always includes `code`, `message`, and `data`; failure data is `null`.
- Best-effort compensation/failed-status persistence must never replace the primary business or infrastructure error. Log the secondary failure or combine both with `errors.Join` when the caller must know both.
- Constructors fail before serving traffic. Startup errors must name the configuration key or construction step but never include credentials.

### 4. Validation & Error Matrix

| Condition | Required behavior |
|---|---|
| Invalid request/business argument | Wrap `ErrInvalidArgument`; HTTP 400/A000001 |
| Resource absent | Wrap `ErrNotFound`; HTTP 404/A000001 |
| Active duplicate or PostgreSQL 23505 | Wrap `ErrDuplicate`; HTTP 400/A000001 |
| Processing ownership not acquired | Wrap `ErrAlreadyRunning`; no slow work |
| Illegal aggregate state change | Wrap `ErrInvalidTransition`; preserve previous aggregate |
| External API/database/storage failure | Wrap operation; HTTP 500/B000001 with generic client message |
| Upload compensation also fails | Return original create error; log compensation error |
| Mark-failed persistence also fails | Return original processing error; log persistence error |
| Server run and database close both fail | Return `errors.Join` containing both chains |

### 5. Good / Base / Bad Cases

- Good: `fmt.Errorf("create document: %w", domain.ErrDuplicate)` still maps through `errors.Is` after multiple layers.
- Base: a missing document returns 404 with all three result fields.
- Bad: compare `err.Error()` with a literal, return a DSN in an error, or overwrite an Embedding failure with a cleanup failure.

### 6. Tests Required

- Domain tests assert invalid transitions leave original values/maps unchanged.
- Application tests use `errors.Is` to assert the primary error survives compensation and failed-status persistence failures.
- HTTP tests assert status, business code, stable envelope, and that unknown error details are not returned to clients.
- Composition-root/server tests assert primary and close/shutdown errors remain discoverable through `errors.Is`.
- Configuration/database tests assert error messages name keys/steps but do not contain DSN secrets.

### 7. Wrong vs Correct

#### Wrong

```go
if err.Error() == "not found" {
    c.JSON(404, Failure("A000001", err.Error()))
}
```

#### Correct

```go
if errors.Is(err, domain.ErrNotFound) {
    c.JSON(http.StatusNotFound, Failure(ClientErrorCode, err.Error()))
    return
}
logger.ErrorContext(ctx, "request failed", "error", err)
c.JSON(http.StatusInternalServerError, Failure(ServiceErrorCode, "internal server error"))
```
