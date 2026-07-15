# TravelAgent 纯 Go 企业级重构实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: use `superpowers:executing-plans` for task-by-task execution. This repository is configured for Codex inline execution; do not dispatch implementation or check sub-agents. Before editing, load `trellis-before-dev`; for every behavior change, follow `superpowers:test-driven-development`.

**Goal:** 删除 Java/Maven 实现，把 Go 模块提升到仓库根目录，并将知识文档服务重构为带轻量 DDD 边界、构造器注入、生产运行保障和中文大白话注释的 Go 单服务项目。

**Architecture:** `knowledge` 是限界上下文，分为 `domain / application / adapter`；接口由 `application` 使用方定义，PostgreSQL、对象存储和 Embedding 位于外层。`internal/app` 是唯一组合根，使用构造器手工注入并管理 HTTP 生命周期。

**Tech Stack:** Go 1.26、Gin 1.11、sqlx、pgx v5、AWS SDK for Go v2、PostgreSQL、pgvector、标准库 `log/slog`、`net/http`、`os/signal`。

---

## 0. 执行规则与范围说明

本计划是一个交叉依赖较强的单体迁移，不拆成 Trellis 子任务：根模块提升、导入路径更新、DDD 拆包、Java 删除和文档同步必须按顺序落地，单独启动任一子任务都会长时间留下不可构建的仓库。

执行约束：

- 每个任务先写或迁移测试，再做最小实现，再运行目标包测试。
- 每个任务结束运行 `go test` 覆盖受影响包；跨包移动后运行 `go test ./...`。
- 生产代码在创建时同步写中文注释，不把注释积压到最后。
- 本计划不执行数据库 SQL，不触碰现有 PostgreSQL 数据。
- 本计划不自动提交 Git；提交动作留到 Trellis Phase 3，并且必须获得用户明确授权。
- 删除未跟踪本地文件前再次解析绝对路径，确认目标位于当前仓库内。

## 1. 文件职责锁定

计划创建或迁移的核心文件：

| 目标文件 | 职责 |
|---|---|
| `cmd/travel-agent/main.go` | 进程信号、调用应用入口、退出码 |
| `internal/app/app.go` | 手工构造数据库、适配器、应用服务和 Handler |
| `internal/app/server.go` | `http.Server` 启停、错误通道和有期限 Shutdown |
| `internal/platform/config/config.go` | 环境变量、默认值、duration 解析和集中校验 |
| `internal/platform/database/postgres.go` | PostgreSQL 建连、Ping 和关闭 |
| `internal/platform/embedding/client.go` | OpenAI 兼容 Embedding HTTP 客户端 |
| `internal/platform/storage/local.go` | 本地对象存储实现 |
| `internal/platform/storage/s3.go` | S3/RustFS 对象存储实现 |
| `internal/platform/httpserver/middleware.go` | 请求 ID、结构化访问日志和 recovery 组装 |
| `internal/knowledge/domain/document.go` | Document 聚合、metadata 和状态转换 |
| `internal/knowledge/domain/chunk.go` | Chunk、ChunkOptions 和分块不变量 |
| `internal/knowledge/domain/status.go` | 文档状态、来源类型和合法性校验 |
| `internal/knowledge/domain/errors.go` | 稳定领域错误 |
| `internal/knowledge/application/ports.go` | 应用使用方定义的 Repository/Storage/Embedder 接口 |
| `internal/knowledge/application/upload_document.go` | 上传、去重、对象存储和补偿 |
| `internal/knowledge/application/process_document.go` | 原子抢占、解析、分块、Embedding 和失败回写 |
| `internal/knowledge/application/query_document.go` | 获取、分页和删除用例 |
| `internal/knowledge/application/chunker.go` | 纯文本分块算法 |
| `internal/knowledge/adapter/http/*.go` | Gin 路由、请求/响应 DTO、Handler 和错误映射 |
| `internal/knowledge/adapter/postgres/*.go` | 行模型、显式转换、SQL 与事务 |
| `migrations/*.sql` | 空库 baseline 与非破坏性升级脚本 |

## 2. Task 1：建立改造前基线与安全清单

**Files:**

- Inspect: `go/**/*.go`
- Inspect: `bootstrap/**`, `framework/**`, `pom.xml`
- Inspect: `resources/database/*.sql`
- Inspect: `README.md`, `AGENTS.md`, `CLAUDE.md`, `.gitignore`

- [x] **Step 1：记录 Git 与任务状态**

运行：

```powershell
git status --short
python ./.trellis/scripts/task.py current --source
```

期望：当前任务为 `.trellis/tasks/07-13-go-enterprise-structure-comments`，除该任务目录外没有未知代码改动。

- [x] **Step 2：运行旧 Go 模块基线测试**

从 `go/` 执行：

```powershell
$env:GOTELEMETRY='off'
$env:GOCACHE='<repo>/.trellis/workspace/go-enterprise-build-cache'
$env:GOMODCACHE='<repo>/.trellis/workspace/go-mod-cache'
go test ./...
```

已验证基线：`config`、`embedding`、`http`、`knowledge` 测试通过；`cmd`、`database`、`storage` 可编译且暂无测试。Codex 沙箱内 Go 子进程写缓存可能需要批准，用户本地终端不需要该沙箱权限。

- [x] **Step 3：冻结外部兼容清单**

在执行日志中记录以下不可变化内容：

```text
POST   /api/knowledge/bases/:kbID/documents/upload
POST   /api/knowledge/documents/:docID/chunk
GET    /api/knowledge/documents/:docID
GET    /api/knowledge/documents/:docID/status
GET    /api/knowledge/bases/:kbID/documents
DELETE /api/knowledge/documents/:docID

response: code / message / data
schema: rag
embedding dimensions: 1536
```

**Checkpoint:** 不修改文件；如果基线不通过，停止迁移并先诊断旧代码。

## 3. Task 2：机械提升 Go 模块到仓库根目录

**Files:**

- Move: `go/go.mod` -> `go.mod`
- Move: `go/go.sum` -> `go.sum`
- Move: `go/.env.example` -> `.env.example`
- Move: `go/cmd/` -> `cmd/`
- Move: `go/internal/` -> `internal/`
- Modify: `go.mod`
- Modify: all moved `.go` imports
- Modify: `.gitignore`

- [x] **Step 1：执行纯机械移动，不同时拆包**

移动后仍暂时保留原包名：

```text
cmd/travel-agent
internal/config
internal/database
internal/embedding
internal/http
internal/knowledge
internal/storage
```

移动前使用 `Resolve-Path` 验证源和目标都位于仓库根目录；如果目标已存在则停止，不覆盖。

- [x] **Step 2：修改模块路径**

`go.mod` 第一行改为：

```go
module github.com/luqingjia/TravelAgent
```

所有内部导入从：

```go
"travel-agent-go/internal/knowledge"
```

改为：

```go
"github.com/luqingjia/TravelAgent/internal/knowledge"
```

- [x] **Step 3：先验证机械迁移没有改变行为**

从仓库根目录运行：

```powershell
New-Item -ItemType Directory -Force .trellis/workspace/bin | Out-Null
go test ./...
go build -o .trellis/workspace/bin/travel-agent.exe ./cmd/travel-agent
```

期望：测试结果与 Task 1 相同，构建产生临时验证二进制但不出现在 Git 状态中。

- [x] **Step 4：更新 Go 忽略项**

`.gitignore` 删除 Maven、Java IDE、旧 `go/.cache` 专属规则，保留或新增：

```gitignore
.env
.data/
.cache/
/travel-agent
/travel-agent.exe
/.trellis/workspace/go-*/
/.trellis/workspace/bin/
```

**Checkpoint:** 此时仓库应仍具备旧包结构，但所有 Go 命令已经从根目录运行。失败时只回滚路径和模块名，不进入 DDD 拆包。

## 4. Task 3：用测试建立轻量 DDD 领域模型

**Files:**

- Create: `internal/knowledge/domain/status.go`
- Create: `internal/knowledge/domain/errors.go`
- Create: `internal/knowledge/domain/document.go`
- Create: `internal/knowledge/domain/document_test.go`
- Create: `internal/knowledge/domain/chunk.go`
- Create: `internal/knowledge/domain/chunk_test.go`
- Source: `internal/knowledge/types.go`

- [x] **Step 1：先写 Document 状态转换失败测试**

测试至少覆盖：

```go
func TestDocumentMarkFailedPreservesMetadata(t *testing.T) {
    doc := domain.Document{
        ID:       "doc-1",
        Status:   domain.StatusProcessing,
        Metadata: map[string]any{"source": "manual"},
    }

    failed, err := doc.MarkFailed("embedding timeout", time.Unix(10, 0))
    if err != nil {
        t.Fatalf("MarkFailed() error = %v", err)
    }
    if failed.Status != domain.StatusFailed {
        t.Fatalf("status = %q", failed.Status)
    }
    if failed.Metadata["source"] != "manual" || failed.Metadata["lastError"] != "embedding timeout" {
        t.Fatalf("metadata = %#v", failed.Metadata)
    }
}
```

同时测试：新文档必须是 `pending`、完成时只删除 `lastError`、负数 chunk count 被拒绝、非法状态被拒绝。

- [x] **Step 2：运行领域测试确认先失败**

```powershell
go test ./internal/knowledge/domain -run TestDocument -v
```

期望：FAIL，原因是领域类型或方法尚不存在。

- [x] **Step 3：实现最小领域 API**

锁定类型与方法：

```go
type DocumentStatus string

const (
    StatusPending    DocumentStatus = "pending"
    StatusProcessing DocumentStatus = "processing"
    StatusCompleted  DocumentStatus = "completed"
    StatusFailed     DocumentStatus = "failed"
)

type NewDocument struct {
    ID            string
    KbID          string
    Title         string
    SourceType    SourceType
    SourceURI     string
    FileName      string
    FileType      string
    FileSize      int64
    ContentHash   string
    Language      string
    ChunkStrategy string
    ChunkConfig   map[string]any
    Metadata      map[string]any
}

type Document struct {
    ID            string
    KbID          string
    Title         string
    SourceType    SourceType
    SourceURI     string
    FileName      string
    FileType      string
    FileSize      int64
    ContentHash   string
    Language      string
    Status        DocumentStatus
    ChunkCount    int
    ChunkStrategy string
    ChunkConfig   map[string]any
    Metadata      map[string]any
    CreateTime    time.Time
    UpdateTime    time.Time
}

func NewPendingDocument(input NewDocument, now time.Time) (Document, error)
func RestoreDocument(snapshot Document) (Document, error)
func (d Document) MarkProcessing(now time.Time) (Document, error)
func (d Document) MarkCompleted(chunkCount int, chunkConfig map[string]any, now time.Time) (Document, error)
func (d Document) MarkFailed(message string, now time.Time) (Document, error)
```

方法返回新值而不是原地修改调用方持有的对象；map 必须复制，防止 metadata 在层间共享后被意外改写。

- [x] **Step 4：迁移 Chunk 与 ChunkOptions**

保留现有 JSON 行为所需字段，但领域类型不携带 `json`/`db` tag。新增：

```go
func (o ChunkOptions) Normalize() (ChunkOptions, error)
func (o ChunkOptions) AsMap() map[string]any
func (c Chunk) Validate() error
```

- [x] **Step 5：运行领域测试**

```powershell
go test ./internal/knowledge/domain -v
```

期望：PASS。

**Checkpoint:** 领域包只导入标准库；执行图谱/源码检查确认不存在 Gin、sqlx、AWS SDK 和 config 导入。

## 5. Task 4：定义应用端口和可测试的 Service

**Files:**

- Create: `internal/knowledge/application/ports.go`
- Create: `internal/knowledge/application/service.go`
- Create: `internal/knowledge/application/test_fakes_test.go`
- Source: `internal/knowledge/types.go`
- Source: `internal/embedding/embedding.go`

- [x] **Step 1：定义使用方接口**

`ports.go` 使用领域类型，并保持接口最小：

```go
type DocumentRepository interface {
    KnowledgeBaseExists(context.Context, string) (bool, error)
    ActiveDocumentHashExists(context.Context, string, string) (bool, error)
    CreateDocument(context.Context, domain.Document) error
    GetDocument(context.Context, string) (domain.Document, error)
    ListDocuments(context.Context, string, int, int) ([]domain.Document, int64, error)
    DeleteDocument(context.Context, string) error
    TryMarkProcessing(context.Context, string) (domain.Document, bool, error)
    ReplaceDocumentChunks(context.Context, domain.Document, []domain.Chunk, [][]float32) error
    MarkFailed(context.Context, domain.Document) error
}

type StoredObjectInput struct {
    FileName    string
    ContentType string
    Content     []byte
}

type StoredObject struct {
    URI         string
    FileName    string
    ContentType string
    Size        int64
}

type ObjectStorage interface {
    Put(context.Context, StoredObjectInput) (StoredObject, error)
    Get(context.Context, string) ([]byte, error)
    Delete(context.Context, string) error
}

type Embedder interface {
    EmbedTexts(context.Context, []string) ([][]float32, error)
}
```

- [x] **Step 2：建立显式构造器**

```go
type Service struct {
    repo     DocumentRepository
    storage  ObjectStorage
    embedder Embedder
    policy   UploadPolicy
    logger   *slog.Logger
    now      func() time.Time
    newID    func() string
}

type UploadPolicy struct {
    MaxUploadBytes    int64
    AllowedExtensions map[string]struct{}
}

type Dependencies struct {
    Repository DocumentRepository
    Storage    ObjectStorage
    Embedder   Embedder
    Policy     UploadPolicy
    Logger     *slog.Logger
    Now        func() time.Time
    NewID      func() string
}

func NewService(deps Dependencies) (*Service, error)
```

`NewService` 对 nil 的 repo/storage/embedder/logger 以及非法上传策略返回明确错误。生产传入真实 clock/ID，测试传入固定函数，避免全局变量。

- [x] **Step 3：测试构造器依赖校验**

```powershell
go test ./internal/knowledge/application -run TestNewService -v
```

期望：缺少 repo/storage/embedder 任一依赖均失败，完整 fake 依赖成功。

**Checkpoint:** 不创建 DI 容器；所有 fake 通过接口直接传给构造函数。

## 6. Task 5：迁移上传用例并保持补偿语义

**Files:**

- Create: `internal/knowledge/application/upload_document.go`
- Create: `internal/knowledge/application/upload_document_test.go`
- Source: `internal/knowledge/service.go:35`
- Source: `internal/knowledge/service_test.go`

- [x] **Step 1：迁移并扩充上传测试**

测试表必须覆盖：

```text
空知识库 ID -> ErrInvalidArgument，storage 未调用
nil/空文件 -> ErrInvalidArgument，storage 未调用
超出最大值 -> ErrInvalidArgument，storage 未调用
不允许扩展名 -> ErrInvalidArgument，storage 未调用
知识库不存在 -> ErrNotFound
内容重复 -> ErrDuplicate，storage 未调用
对象写入成功但 CreateDocument 失败 -> 调用 Delete 一次并返回原错误
合法上传 -> pending，chunkCount=0，SHA-256 正确
```

- [x] **Step 2：运行上传测试确认失败**

```powershell
go test ./internal/knowledge/application -run TestUploadDocument -v
```

- [x] **Step 3：迁移最小实现**

保留限长读取：

```go
content, err := io.ReadAll(io.LimitReader(input.Content, policy.MaxUploadBytes+1))
```

构造 pending 聚合必须调用 `domain.NewPendingDocument`。数据库创建失败时：

```go
if err := s.repo.CreateDocument(ctx, document); err != nil {
    if cleanupErr := s.storage.Delete(ctx, stored.URI); cleanupErr != nil {
        s.logger.ErrorContext(ctx, "compensate stored object", "error", cleanupErr)
    }
    return domain.Document{}, fmt.Errorf("create document: %w", err)
}
```

补偿错误只记录，不能覆盖原始错误。

- [x] **Step 4：运行上传和领域测试**

```powershell
go test ./internal/knowledge/domain ./internal/knowledge/application -run 'Test(UploadDocument|Document)' -v
```

期望：PASS。

## 7. Task 6：迁移分块、Embedding 和失败回写用例

**Files:**

- Create: `internal/knowledge/application/chunker.go`
- Create: `internal/knowledge/application/chunker_test.go`
- Create: `internal/knowledge/application/process_document.go`
- Create: `internal/knowledge/application/process_document_test.go`
- Source: `internal/knowledge/chunker.go`
- Source: `internal/knowledge/service.go:118`

- [x] **Step 1：先迁移分块算法测试**

保留现有顺序、位置和内容断言，并增加空文本、非法 options、超长段落测试。

```powershell
go test ./internal/knowledge/application -run TestChunkText -v
```

期望：首次运行 FAIL，迁移算法后 PASS。

- [x] **Step 2：写处理流程测试**

覆盖：

```text
原子抢占失败 -> ErrAlreadyRunning，不读对象
Storage.Get 失败 -> MarkFailed，返回原错误
解析/分块为空 -> MarkFailed，不调用 Embedder/Replace
Embedding 数量不匹配 -> MarkFailed，不 Replace
任一向量不是 1536 维 -> MarkFailed，不 Replace
成功 -> Replace 一次，Document completed，清除 lastError 并保留其他 metadata
Replace 失败 -> 返回原错误并尝试 MarkFailed
```

- [x] **Step 3：实现处理编排**

固定顺序：

```go
document, acquired, err := s.repo.TryMarkProcessing(ctx, docID)
// Storage.Get -> ExtractText -> ChunkText -> EmbedTexts
// validate vector count and dimensions
completed, err := document.MarkCompleted(len(chunks), normalizedOptions.AsMap(), s.now())
// only then call ReplaceDocumentChunks
```

所有慢操作位于事务外；只有 PostgreSQL 适配器的 Replace 方法开启事务。

任一处理步骤失败时，先让聚合生成合法的 `failed` 状态，再把整个聚合交给仓储持久化；失败回写自身出错只记日志，不能覆盖真正的处理错误：

```go
failed, transitionErr := document.MarkFailed(processErr.Error(), s.now())
if transitionErr == nil {
    if persistErr := s.repo.MarkFailed(ctx, failed); persistErr != nil {
        s.logger.ErrorContext(ctx, "persist failed document status", "error", persistErr)
    }
}
return domain.Document{}, processErr
```

- [x] **Step 4：运行应用层全量测试**

```powershell
go test ./internal/knowledge/application -v
```

期望：PASS。

## 8. Task 7：建立 PostgreSQL 适配器和显式模型转换

**Files:**

- Create: `internal/knowledge/adapter/postgres/model.go`
- Create: `internal/knowledge/adapter/postgres/model_test.go`
- Create: `internal/knowledge/adapter/postgres/repository.go`
- Create: `internal/knowledge/adapter/postgres/vector.go`
- Create: `internal/knowledge/adapter/postgres/vector_test.go`
- Source: `internal/knowledge/repository_sql.go`
- Source: `internal/embedding/vector_test.go`

- [x] **Step 1：先写数据库行模型双向转换测试**

`documentRow` 保留 `db` tag；转换测试逐字段比较 ID、KbID、状态、chunkConfig、metadata 和时间，防止拆层丢字段。

```go
func TestDocumentRowRoundTrip(t *testing.T) {
    original := completeDomainFixture()
    row, err := rowFromDomain(original)
    // marshal/scan JSON fields, then restore
    restored, err := row.toDomain()
    // assert every field
}
```

- [x] **Step 2：迁移 pgvector 文本测试与实现**

`vectorText` 位于 PostgreSQL 适配器，因为 `::vector` 是持久化格式。测试空向量报错、普通值格式和 1536 维长度校验。

- [x] **Step 3：迁移 Repository SQL**

构造器和接口断言：

```go
type Repository struct { db *sqlx.DB }

func NewRepository(db *sqlx.DB) (*Repository, error)

var _ application.DocumentRepository = (*Repository)(nil)
```

SQL 表名、逻辑删除条件、唯一冲突 `23505` 映射和分页语义保持不变。

- [x] **Step 4：保留原子替换事务**

`ReplaceDocumentChunks` 必须按以下顺序使用同一个 `sqlx.Tx`：

```text
DELETE old vectors
DELETE old chunks
INSERT new chunks
INSERT/UPSERT new vectors
UPDATE document completed
COMMIT
```

使用具名返回错误或显式 `committed` 标记保证所有提前返回都会 Rollback；不能依赖被遮蔽的局部 `err`。

- [x] **Step 5：运行适配器和应用测试**

```powershell
go test ./internal/knowledge/adapter/postgres ./internal/knowledge/application -v
```

期望：PASS。

## 9. Task 8：迁移 platform 配置、数据库、存储和 Embedding

**Files:**

- Create: `internal/platform/config/config.go`
- Create: `internal/platform/config/config_test.go`
- Create: `internal/platform/database/postgres.go`
- Create: `internal/platform/embedding/client.go`
- Create: `internal/platform/embedding/client_test.go`
- Create: `internal/platform/storage/local.go`
- Create: `internal/platform/storage/s3.go`
- Delete after migration: old `internal/config`, `internal/database`, `internal/embedding`, `internal/storage`

- [x] **Step 1：先写配置校验和 duration 测试**

测试默认值：

```text
port=8081
readHeaderTimeout=5s
readTimeout=60s
writeTimeout=5m
idleTimeout=60s
shutdownTimeout=15s
logLevel=info
logFormat=json
embeddingDimensions=1536
```

测试非法 duration、缺少 DSN、启用 S3 时缺 bucket/key、缺 Embedding API key，并测试本地存储模式不要求 S3 凭据。

- [x] **Step 2：实现 Load 与 Validate 分离**

```go
func Load(getenv func(string) string) (Config, error)
func (c Config) Validate() error
```

测试传入 map-backed `getenv`，不并发修改进程环境。

- [x] **Step 3：迁移数据库构造器**

```go
func Open(ctx context.Context, cfg config.Database) (*sqlx.DB, error)
```

设置连接池参数，调用 `PingContext`；错误用 `%w` 包装并且不得打印 DSN 密码。

- [x] **Step 4：迁移存储与 Embedding 实现**

增加编译期接口断言：

```go
var _ application.ObjectStorage = (*LocalStorage)(nil)
var _ application.ObjectStorage = (*S3Storage)(nil)
var _ application.Embedder = (*Client)(nil)
```

保留 S3 URI 解析、endpoint、path-style 以及 Embedding 请求/响应协议。

- [x] **Step 5：运行 platform 测试**

```powershell
go test ./internal/platform/... -v
```

期望：PASS，测试不访问真实 PostgreSQL、S3 或 Embedding API。

## 10. Task 9：建立 HTTP 适配器和请求级中间件

**Files:**

- Create: `internal/knowledge/adapter/http/request.go`
- Create: `internal/knowledge/adapter/http/response.go`
- Create: `internal/knowledge/adapter/http/handler.go`
- Create: `internal/knowledge/adapter/http/router.go`
- Create: `internal/knowledge/adapter/http/handler_test.go`
- Create: `internal/platform/httpserver/request_id.go`
- Create: `internal/platform/httpserver/middleware.go`
- Create: `internal/platform/httpserver/middleware_test.go`
- Source: old `internal/http/*.go`

- [x] **Step 1：先写响应兼容测试**

使用 `httptest` 验证成功和失败仍为：

```json
{"code":"0","message":"","data":{}}
```

并验证领域错误到 HTTP 状态/业务 code 的稳定映射。

- [x] **Step 2：写请求 ID 与访问日志测试**

测试：

- 无 `X-Request-ID` 时生成非空值并回传响应头；
- 合法请求 ID 被沿用；超长值被替换；
- 日志记录 `request_id/method/path/status/latency`；
- panic 被 recovery 转换为 500 且不泄漏堆栈。

- [x] **Step 3：实现 Handler 构造器注入**

```go
type KnowledgeService interface {
    UploadDocument(context.Context, application.UploadInput) (domain.Document, error)
    ProcessDocument(context.Context, string, domain.ChunkOptions) (domain.Document, error)
    GetDocument(context.Context, string) (domain.Document, error)
    ListDocuments(context.Context, string, int, int) ([]domain.Document, int64, error)
    DeleteDocument(context.Context, string) error
}

func NewHandler(service KnowledgeService, logger *slog.Logger) (*Handler, error)
```

不得通过 `gin.Context` 获取 service/repository/db。

- [x] **Step 4：注册原路径**

```go
func NewRouter(handler *Handler, middleware httpserver.Middleware) *gin.Engine
```

精确注册基线中的六条 `/api/knowledge` 路由和 `/health`。

- [x] **Step 5：运行 HTTP 测试**

```powershell
go test ./internal/knowledge/adapter/http ./internal/platform/httpserver -v
```

期望：PASS。

## 11. Task 10：建立 app 组合根、HTTP 生命周期和薄 main

**Files:**

- Create: `internal/app/app.go`
- Create: `internal/app/app_test.go`
- Create: `internal/app/server.go`
- Create: `internal/app/server_test.go`
- Modify: `cmd/travel-agent/main.go`

- [x] **Step 1：先写 HTTP Server 生命周期测试**

使用 `net.Listen("tcp", "127.0.0.1:0")`，验证：

- Serve 启动错误会返回；
- context 取消后调用 Shutdown；
- shutdown 超时返回包装错误；
- 测试不占用固定 8081 端口。

- [x] **Step 2：实现 Server**

```go
type Server struct {
    httpServer      *http.Server
    shutdownTimeout time.Duration
    logger          *slog.Logger
}

func NewServer(cfg config.HTTP, handler http.Handler, logger *slog.Logger) (*Server, error)
func (s *Server) Run(ctx context.Context) error
```

`Run` 同时等待 Serve 错误和 context 取消；`http.ErrServerClosed` 视为正常关闭。

- [x] **Step 3：实现 app 手工组装**

`app.New` 按固定顺序创建 logger、db、repo、storage、embedder、service、handler、router、server。任何中途失败都关闭已经创建的资源。

不允许：

```go
c.Set("service", service)
globalDB = db
container.Resolve(&handler)
```

- [x] **Step 4：把 main 压缩为进程入口**

目标形状：

```go
func main() {
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    if err := app.Run(ctx); err != nil {
        slog.Error("travel-agent stopped", "error", err)
        os.Exit(1)
    }
}
```

- [x] **Step 5：运行全量 Go 验证**

```powershell
go test ./...
go vet ./...
go build -o .trellis/workspace/bin/travel-agent.exe ./cmd/travel-agent
```

期望：全部通过。

## 12. Task 11：迁移 SQL 并删除 Java/Maven 与旧 Go 包

**Files:**

- Move: `resources/database/rag.sql` -> `migrations/000001_rag_baseline.sql`
- Move: `resources/database/knowledge-ingestion-mvp-upgrade.sql` -> `migrations/000002_knowledge_ingestion_upgrade.sql`
- Delete: `bootstrap/`
- Delete: `framework/`
- Delete: `pom.xml`
- Delete: old empty `go/`, `resources/`, and obsolete old Go packages
- Delete local/untracked: `doc/knowledge-implementation-plan.md`
- Delete local/untracked: `.github/modernize/java-upgrade/`

- [x] **Step 1：移动 SQL，不执行 SQL**

在 baseline 顶部增加醒目说明：

```sql
-- 仅用于全新空数据库初始化。
-- 本文件包含重建语句，禁止在已有业务数据的数据库上直接执行。
```

upgrade 保持非破坏性重复检查和 1536 维约束。

- [x] **Step 2：删除 tracked Java 内容**

删除前确认：

```powershell
git ls-files -- bootstrap framework pom.xml
```

删除后确认输出为空，并确认 `git diff --name-status` 只包含预期删除/移动。

- [x] **Step 3：删除已获用户批准的本地 Java 资料**

分别 `Resolve-Path`，确认最终绝对路径以仓库根目录开头，再删除：

```text
<repo>/doc/knowledge-implementation-plan.md
<repo>/.github/modernize/java-upgrade
```

不得递归删除 `<repo>/doc`、`<repo>/.github` 或任何计算后未核验的父目录。

- [x] **Step 4：运行 Go 全量测试**

```powershell
go test ./...
go build -o .trellis/workspace/bin/travel-agent.exe ./cmd/travel-agent
```

期望：删除 Java 后 Go 仍全部通过。

**Rollback:** SQL 移动和 tracked Java 删除可从 Git 恢复；未跟踪本地资料不可从 Git 恢复，已由用户明确批准。

## 13. Task 12：重写项目说明、开发规范和 Trellis 规范

**Files:**

- Modify: `README.md`
- Modify: `AGENTS.md`
- Modify: `CLAUDE.md`
- Modify: `.gitignore`
- Modify: `.trellis/spec/backend/index.md`
- Modify: `.trellis/spec/backend/directory-structure.md`
- Modify: `.trellis/spec/backend/database-guidelines.md`
- Modify: `.trellis/spec/backend/error-handling.md`
- Modify: `.trellis/spec/backend/logging-guidelines.md`
- Modify: `.trellis/spec/backend/quality-guidelines.md`

- [x] **Step 1：重写 README 为纯 Go 运行手册**

必须包含：项目定位、真实目录、轻量 DDD 边界、依赖方向、环境变量、PostgreSQL/pgvector 要求、空库 SQL 警告、根目录启动/测试/构建命令、当前支持格式和 MVP 边界。

- [x] **Step 2：重写 AGENTS/CLAUDE 当前规范**

删除 Spring、Maven、Java 21、MyBatis、构造器注入 Lombok 等现行说明，改为 Go 包边界、构造器手工注入、错误包装、context 首参、测试和注释规则。保留 Trellis managed block。

- [x] **Step 3：通过 `trellis-update-spec` 更新后端规范**

规范必须变成可执行契约：

```text
domain only stdlib
application owns interfaces
adapters depend inward
app is composition root
explicit pgvector cast and one replacement transaction
slog fields and no secret logging
root go test/vet/build gates
```

- [x] **Step 4：全仓当前说明扫描**

排除 `.trellis/tasks/archive/` 后搜索 Java/Maven/Spring/旧 `go/` 路径；每个命中必须属于历史说明、SQL 注释或被改写，不允许当前运行文档继续引用已删除结构。

## 14. Task 13：中文大白话注释专项审计

**Files:** all production `.go` files and all `_test.go` files under `cmd/` and `internal/`.

- [x] **Step 1：生产代码逐包审计**

每个生产包确认：

- package comment 说明职责和不能做什么；
- 导出类型、接口、函数有符合 GoDoc 的中文注释；
- 构造函数解释依赖为什么必需；
- 业务用例按顺序解释校验、状态变化、外部调用、事务边界和补偿；
- context、defer、错误包装、接口隐式实现、pgvector cast、map 复制有大白话解释；
- 没有“给变量赋值”“进入 if”这类机械注释。

- [x] **Step 2：测试代码审计**

每个测试说明场景风险，并用注释标识准备、执行和关键断言的业务意义。Fake 的失败开关要解释它模拟的真实故障。

- [x] **Step 3：格式化并运行测试**

```powershell
go fmt ./...
go test ./...
go vet ./...
```

期望：PASS；`gofmt` 后重新查看 diff，确认注释没有脱离对应语句。

## 15. Task 14：最终质量门与审查交付

**Files:** entire repository, excluding historical `.trellis/tasks/archive/` for current-architecture scans.

- [x] **Step 1：运行完整验证**

```powershell
New-Item -ItemType Directory -Force .trellis/workspace/bin | Out-Null
go test ./...
go vet ./...
go build -o .trellis/workspace/bin/travel-agent.exe ./cmd/travel-agent
git diff --check
git status --short
```

期望：测试、vet、build、diff check 全部成功；Git 状态只包含计划内代码、文档、任务文件和 Java 删除。

- [x] **Step 2：结构验证**

确认根目录存在：

```text
cmd/
internal/
migrations/
go.mod
go.sum
.env.example
```

确认不存在：

```text
bootstrap/
framework/
go/
pom.xml
*.java
```

- [x] **Step 3：依赖图验证**

刷新 codebase-memory 索引并检查：

- `internal/knowledge/domain` 无第三方导入；
- `application` 不导入 Gin、sqlx、pgx、AWS SDK；
- `adapter/http` 不导入 PostgreSQL/存储实现；
- 只有 `internal/app` 同时依赖具体适配器。

- [x] **Step 4：兼容性抽查**

使用 `httptest` 或本地临时端口确认 `/health` 和六条知识接口仍注册；检查响应 JSON 字段、默认端口、1536 维校验和 SQL 表名没有漂移。

- [x] **Step 5：凭据与生成物检查**

确认 `.env.example` 只有变量名/安全示例，日志与 diff 中没有真实 API key、数据库密码或 S3 secret；确认临时二进制和 Go cache 被忽略。

- [x] **Step 6：Trellis 最终检查**

加载 `trellis-check`，对 PRD 条目逐项给出验证证据。存在失败则回到对应 Task 修复，不得把任务标记完成。

## 16. 最终审批门

实施完成且所有质量门通过后：

1. 向用户汇报具体变更、删除范围和验证输出。
2. 不自动提交；先让用户审核工作区 diff。
3. 只有用户明确要求提交时，才进入 Trellis Phase 3 提交步骤。
4. 建议提交信息：`refactor(go): standardize DDD project structure`。
