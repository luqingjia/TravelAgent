# Logging Guidelines

## Scenario: Record process, request, and secondary-failure diagnostics

### 1. Scope / Trigger

- Trigger: adding logs, middleware, external clients, lifecycle events, compensation, or configuration.
- Goal: produce structured, correlatable diagnostics without leaking credentials or document content.

### 2. Signatures

```go
func newLogger(cfg config.Log) (*slog.Logger, error)
func httpserver.NewMiddleware(logger *slog.Logger) (httpserver.Middleware, error)
```

Required access-log fields:

```text
request_id, method, path, status, latency_ms
```

Configuration:

```text
LOG_LEVEL=debug|info|warn|error
LOG_FORMAT=json|text
```

### 3. Contracts

- Use Go standard-library `log/slog`; do not introduce a second logging facade.
- Production default is JSON at info level; local development may use text.
- Use `InfoContext`/`ErrorContext` when a request or use-case context exists.
- Request ID middleware validates or generates `X-Request-ID`, stores it as request-scoped data, and returns it in the response header.
- Access logs run after handlers so status and latency are final.
- Panic recovery records request metadata server-side and returns an empty 500 without stack or panic text.
- Log startup/shutdown address and timeout, primary external-operation failures, and secondary compensation/persistence failures.
- Never log `POSTGRESQL_DSN`, Authorization headers, API keys, S3 access/secret keys, full uploaded bytes, or full third-party response bodies.

### 4. Validation & Error Matrix

| Condition | Required behavior |
|---|---|
| Missing request ID | Generate a valid ID and return it in `X-Request-ID` |
| Valid caller request ID | Reuse it |
| Empty/too-long/invalid request ID | Replace it |
| Handler completes | Log required access fields once |
| Handler panics | Log structured panic context; send non-leaking 500 |
| Compensation fails | Log secondary error without replacing primary return error |
| Invalid log level/format | Fail configuration validation before DB/network setup |
| External call fails | Log operation and error chain, never secrets or payload body |

### 5. Good / Base / Bad Cases

- Good: one JSON access event contains request ID, route path, status, and elapsed milliseconds.
- Base: service startup/shutdown emits address and configured timeout.
- Bad: log an entire config struct, DSN, Authorization header, uploaded file, or panic stack to the HTTP client.

### 6. Tests Required

- Middleware tests capture slog output and assert all required fields.
- Tests assert a valid incoming request ID is reused and an overlong value is replaced.
- Recovery tests assert HTTP 500 does not contain panic text or stack data.
- Logger/config tests cover all four levels, both formats, and invalid values.
- Secret scan checks current diffs and examples for real credentials.

### 7. Wrong vs Correct

#### Wrong

```go
logger.Error("database failed", "dsn", cfg.Database.DSN, "error", err)
```

#### Correct

```go
logger.ErrorContext(ctx, "database operation failed",
    "request_id", httpserver.RequestID(c),
    "operation", "create_document",
    "error", err,
)
```
