# Quality Guidelines

## Scenario: Implement and verify a Go backend change

### 1. Scope / Trigger

- Trigger: every behavior change, refactor, package move, documentation/configuration update, or completion claim.
- Goal: keep behavior compatible, comments trustworthy, dependencies inward, and repository state reviewable.

### 2. Signatures

Mandatory root commands:

```powershell
go fmt ./...
go test ./...
go vet ./...
go build -o .trellis/workspace/bin/travel-agent.exe ./cmd/travel-agent
git diff --check
git status --short
```

Stable compatibility surface:

```text
GET    /health
POST   /api/knowledge/bases/:kbID/documents/upload
POST   /api/knowledge/documents/:docID/chunk
GET    /api/knowledge/documents/:docID
GET    /api/knowledge/documents/:docID/status
GET    /api/knowledge/bases/:kbID/documents
DELETE /api/knowledge/documents/:docID
response fields: code, message, data
embedding dimensions: 1536
```

### 3. Contracts

- Follow red-green-refactor for behavior changes: write the smallest failing test, verify the expected failure, implement minimally, verify the target and affected suites, then refactor.
- Tests must explain the scenario, preparation/failure injection, action, and key business assertions in plain Chinese.
- Production packages, exported types/functions, constructors, business steps, transaction boundaries, context propagation, compensation, and non-obvious syntax require accurate plain-Chinese comments.
- Comments explain intent, constraints, data change, and failure consequences; do not translate obvious syntax line by line.
- Run commands from repository root. `.env.example` is not automatically loaded.
- Preserve unrelated user changes and do not commit caches, binaries, local data, or credentials.
- Do not report completion without fresh test/vet/build/diff evidence.

### 4. Validation & Error Matrix

| Condition | Required behavior |
|---|---|
| New behavior has no failing test first | Stop and add a test before production implementation |
| Target test fails for typo/setup error | Fix the test until it fails for the missing behavior |
| Target passes but affected suite fails | Fix regression before continuing |
| Comment contradicts implementation | Update comment or simplify code; never leave stale explanation |
| `go vet`, build, or `git diff --check` fails | Task is not complete |
| Current docs mention removed paths/commands | Rewrite before final review |
| Diff contains credentials/generated output | Remove and re-run status/secret checks |

### 5. Good / Base / Bad Cases

- Good: a domain transition test fails first, the minimal method passes it, package/full tests pass, and comments explain metadata preservation.
- Base: a documentation-only change still passes `git diff --check` and current-reference scans.
- Bad: add tests after implementation, claim success from cached old output, mechanically comment every assignment, or skip build because tests passed.

### 6. Tests Required

- Domain: invariants, state transitions, map-copy isolation.
- Application: validation, orchestration order, no-call assertions, compensation, failure persistence, vector checks.
- Adapters: DTO/row conversion, error mapping, routes, request IDs, SQL/vector formatting, transaction rollback.
- Platform/app: config parsing, secret-safe errors, client protocols, storage URI safety, middleware, assembly order, graceful shutdown.
- Final: all mandatory commands plus architecture/import scan, route compatibility check, structure check, current-doc reference scan, and credential/generated-artifact scan.

### 7. Wrong vs Correct

#### Wrong

```go
// Set status.
document.Status = StatusCompleted
```

#### Correct

```go
// 只有分块和向量都准备完整后才生成 completed 新值；原对象保持不变，
// 这样事务失败时调用方仍能明确区分“已准备完成”和“已持久化完成”。
completed, err := document.MarkCompleted(len(chunks), options.AsMap(), now)
```
