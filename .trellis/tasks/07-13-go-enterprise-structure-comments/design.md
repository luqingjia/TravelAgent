# 纯 Go 企业级目录与轻量 DDD 技术设计

## 1. 设计目标

本次改造把仓库从“Java Maven 多模块 + `go/` 子模块”收敛为一个可从仓库根目录直接构建、测试和运行的 Go 单服务仓库。

目标不是堆叠目录，而是让以下边界可以被代码和测试验证：

- 领域规则不依赖 Gin、sqlx、pgx、AWS SDK 或环境变量。
- 应用用例只编排业务流程，并通过使用方定义的小接口访问外部资源。
- HTTP、PostgreSQL、对象存储和 Embedding 都是可替换的外层适配器。
- `cmd` 入口保持极薄，`app` 是唯一依赖组装位置。
- 保持现有 API、数据库 schema、对象存储语义和 1536 维向量兼容。
- 所有生产代码具有面向初学者的中文大白话注释，业务关键步骤按执行顺序细讲。

## 2. 现状证据

当前实现已经具有清晰的业务主流程，但职责仍集中在少数包中：

- `go/cmd/travel-agent/main.go:15` 同时加载配置、建立数据库、创建存储与 Embedding、组装业务并直接启动 Gin。
- `go/internal/knowledge/types.go:27` 的 `Document` 同时携带领域数据、数据库 tag 和 JSON tag。
- `go/internal/knowledge/types.go:94` 把仓储、存储接口与领域类型放在同一个文件中。
- `go/internal/knowledge/service.go:35` 的上传用例同时负责校验、读取、去重、存储补偿和聚合创建。
- `go/internal/knowledge/service.go:118` 的分块用例负责处理状态抢占、失败回写和完整处理编排。
- `go/internal/knowledge/repository_sql.go:149` 在一个事务内替换 chunk、vector 并更新文档完成状态；这个事务边界必须保留。
- `go/internal/http/router.go:42` 直接把 multipart 请求转换为业务输入并返回领域对象。
- `go/internal/config/config.go:53` 能读取环境变量，但还没有集中校验必要配置和 HTTP 生命周期参数。

## 3. 架构选择

采用“模块化单体 + 轻量 DDD + Ports and Adapters 思想”，但按 Go 习惯落地：接口由使用方定义，不建立独立的全局 `port/` 接口仓库。

### 3.1 DDD 使用范围

当前唯一限界上下文是 `knowledge`。未来出现 Agent 编排、行程规划等真实业务后，再新增平级上下文，例如 `internal/agent`、`internal/itinerary`；本次不创建空目录。

轻量 DDD 包含：

- `Document` 作为当前聚合根，维护处理状态、分块数量和失败元数据规则。
- `Chunk` 作为文档处理产生的领域实体。
- `DocumentStatus`、`SourceType`、`ChunkOptions` 使用明确类型表达业务含义。
- 状态转换和不变量放在领域方法中，不散落在 Handler、SQL 或配置代码里。
- 上传、分块、查询、删除是应用用例；对象存储、Embedding 调用和事务仓储通过小接口访问。

本次不引入：

- CQRS 或事件溯源；
- 领域事件总线；
- 通用仓储基类；
- 为每张数据库表创建聚合；
- Java 风格的 DTO/DO/VO/BO 多重复制体系。

### 3.2 目标目录

```text
TravelAgent/
├── cmd/
│   └── travel-agent/
│       └── main.go
├── internal/
│   ├── app/
│   │   ├── app.go
│   │   └── server.go
│   ├── platform/
│   │   ├── config/
│   │   │   ├── config.go
│   │   │   └── config_test.go
│   │   ├── database/
│   │   │   └── postgres.go
│   │   ├── embedding/
│   │   │   └── client.go
│   │   ├── storage/
│   │   │   ├── local.go
│   │   │   └── s3.go
│   │   └── httpserver/
│   │       ├── middleware.go
│   │       └── request_id.go
│   └── knowledge/
│       ├── domain/
│       │   ├── document.go
│       │   ├── chunk.go
│       │   ├── status.go
│       │   └── errors.go
│       ├── application/
│       │   ├── service.go
│       │   ├── ports.go
│       │   ├── upload_document.go
│       │   ├── process_document.go
│       │   ├── query_document.go
│       │   └── chunker.go
│       └── adapter/
│           ├── http/
│           │   ├── router.go
│           │   ├── handler.go
│           │   ├── request.go
│           │   └── response.go
│           └── postgres/
│               ├── repository.go
│               ├── model.go
│               └── vector.go
├── migrations/
│   ├── 000001_rag_baseline.sql
│   └── 000002_knowledge_ingestion_upgrade.sql
├── .env.example
├── go.mod
├── go.sum
└── README.md
```

目录树表示职责目标，不要求为了“一文件一概念”机械拆出只有几行的文件。实施时相邻的小类型可以合并，但不能重新混合领域、HTTP 和数据库职责。

### 3.3 依赖方向

```text
cmd/travel-agent
      |
      v
internal/app --------------> 具体适配器与 platform 实现
                                  |
HTTP adapter ---> application <-- PostgreSQL / Storage / Embedding
                       |
                       v
                     domain
```

规则：

1. `domain` 只依赖 Go 标准库。
2. `application` 依赖 `domain`，并在 `ports.go` 定义自己需要的最小接口。
3. HTTP 适配器依赖 `application`；PostgreSQL、存储和 Embedding 实现应用接口。
4. `app` 可以导入所有具体实现，因为它是组合根；其他包不得承担全局组装职责。
5. 禁止创建 `common`、`utils`、`models` 等无明确业务边界的兜底包。

### 3.4 依赖注入

Gin 只负责 HTTP 路由和请求上下文，不承担应用依赖管理。项目采用构造器手工注入，`internal/app` 是唯一组合根：

```go
repo := postgres.NewRepository(db)
store := storage.NewS3Storage(storageConfig)
embedder := embedding.NewClient(embeddingConfig)
service, err := application.NewService(application.Dependencies{
    Repository: repo,
    Storage:    store,
    Embedder:   embedder,
    Policy:     uploadPolicy,
    Logger:     logger,
})
handler, err := httpadapter.NewHandler(service, logger)
middleware := httpserver.NewMiddleware(logger)
router := httpadapter.NewRouter(handler, middleware)
```

约束：

- 构造函数接收完成职责所需的最小依赖，不能在函数内部读取全局单例。
- 长期依赖不得通过 `gin.Context.Set/Get` 进行服务定位。
- Gin Context 只保存请求 ID、认证主体、trace 等请求级信息。
- 本任务不引入运行时 DI 容器或代码生成 DI 框架；当未来出现多个可执行程序、大量可选实现或复杂生命周期图时再重新评估。
- 测试直接传入 fake/stub 实现，不需要启动 Gin 或真实基础设施来构造应用服务。

## 4. 领域模型设计

### 4.1 Document 聚合

`Document` 不再携带 `db` 和 `json` tag。HTTP 响应模型与 PostgreSQL 行模型分别在适配层定义并显式转换。

领域方法负责表达：

- 新上传文档以 `pending` 状态创建，`chunkCount = 0`。
- 只有仓储原子抢占成功后，文档才进入 `processing`。
- 成功完成时状态变为 `completed`，更新分块数量并仅清除 `metadata.lastError`。
- 失败时状态变为 `failed`，保留已有 metadata，并覆盖最近一次 `lastError`。
- 非法状态值、负数分块数量、空文档 ID 等不变量在构造或恢复时拒绝。

并发抢占不能只靠内存领域方法。PostgreSQL 适配器仍使用条件更新完成原子状态切换，并把抢占后的聚合返回应用层。

### 4.2 Chunk 与分块规则

`Chunk` 保存内容、顺序和原文位置。分块算法属于知识域规则，但不访问数据库、HTTP 或 Embedding。

`ChunkOptions` 在进入算法前完成默认值和范围归一化；生成的 chunk 必须保持：

- `Index` 连续且从 0 开始；
- `StartPosition <= EndPosition`；
- `Content` 与原文对应区间一致；
- 空白结果不能进入 Embedding 和替换事务。

## 5. 应用用例与数据流

### 5.1 上传文档

```text
HTTP multipart
  -> 请求 DTO 校验
  -> UploadDocument 用例
  -> 校验知识库、大小、扩展名
  -> 限长读取并计算 SHA-256
  -> 重复预检查
  -> 对象存储 Put
  -> 创建 pending Document
  -> Repository.Create
  -> 失败时尽力补偿删除对象
  -> HTTP 响应 DTO
```

对象存储和数据库不是同一个事务资源，因此继续使用补偿策略。补偿失败只记录日志，不能覆盖原始数据库错误。

### 5.2 显式分块

```text
HTTP JSON
  -> ProcessDocument 用例
  -> Repository.TryMarkProcessing 原子抢占
  -> Storage.Get
  -> 文本解析
  -> 领域分块
  -> Embedder.EmbedTexts
  -> 校验数量和每个 1536 维向量
  -> 领域聚合标记 completed
  -> Repository.ReplaceChunks 事务提交
```

解析、分块和 Embedding 都在数据库事务外完成。只有完整且已验证的数据进入事务。任何失败都会尽力调用 `MarkFailed` 保存最近错误，但回写失败不能替换原始处理错误。

### 5.3 查询与删除

查询用例返回领域读模型，由 HTTP 适配器转换为兼容现有 JSON 字段的响应。删除继续保持现有软删除/对象清理语义，不在本任务增加新的级联规则。

## 6. PostgreSQL 与迁移设计

- 继续使用 `sqlx + pgx` 和现有 `rag` schema。
- PostgreSQL 行结构体独立携带 `db` tag；扫描后显式恢复领域对象。
- `ReplaceDocumentChunks` 保持单事务：删除旧 vector、删除旧 chunk、写入新 chunk/vector、更新文档完成状态。
- 向量先在应用层验证为 1536 维，再在适配器转换为 pgvector 文本并使用 `$n::vector` 显式写入。
- 并发重复仍由 `(kb_id, content_hash)` 部分唯一索引兜底，并映射成领域重复错误。
- 本次只移动 SQL 文件，不自动连接数据库或执行迁移，不删除或重建现有业务数据。
- `000001_rag_baseline.sql` 明确标注为“仅用于空数据库初始化”；现有含 `DROP TABLE` 的脚本不得在已有数据库上自动执行。

## 7. HTTP、错误与兼容性

- 保持现有 `/api/knowledge/...` 路径和 `code/message/data` 响应外壳。
- HTTP 请求/响应 DTO 位于 `adapter/http`，领域对象不带 JSON tag。
- 领域错误映射为稳定的客户端状态码和业务 code；未知基础设施错误返回通用服务错误，并在服务端记录完整错误链。
- 所有跨层错误使用 `%w` 包装，错误判断使用 `errors.Is/As`，不能依赖字符串比较。
- 请求日志包含 `request_id`、method、path、status、latency；响应头回传 `X-Request-ID`。
- 保留 Gin recovery，panic 响应不能泄露堆栈或密钥。

## 8. 配置、日志与服务生命周期

配置仍来自环境变量，并增加以下可解析时长：

- `HTTP_READ_HEADER_TIMEOUT`，默认 `5s`；
- `HTTP_READ_TIMEOUT`，默认 `60s`；
- `HTTP_WRITE_TIMEOUT`，默认 `5m`，为同步文档处理保留空间；
- `HTTP_IDLE_TIMEOUT`，默认 `60s`；
- `HTTP_SHUTDOWN_TIMEOUT`，默认 `15s`；
- `LOG_LEVEL`，默认 `info`；
- `LOG_FORMAT`，默认 `json`，本地可设为 `text`。

启动顺序：

1. 读取并校验配置；
2. 创建 `slog.Logger`；
3. 建立数据库并执行 ping；
4. 创建存储与 Embedding 客户端；
5. 手工组装应用服务和适配器；
6. 启动 `http.Server`；
7. 等待 `SIGINT/SIGTERM`；
8. 在 shutdown timeout 内调用 `Server.Shutdown`，最后关闭数据库。

`main.go` 只负责进程级信号和退出；组装细节放在 `internal/app`，便于测试启动失败和关闭路径。

## 9. 文件迁移与删除边界

### 9.1 提升到根目录

- `go/go.mod` -> `go.mod`，模块名改为 `github.com/luqingjia/TravelAgent`。
- `go/go.sum` -> `go.sum`。
- `go/.env.example` -> `.env.example`。
- `go/cmd/` -> `cmd/`，随后按组合根设计调整。
- `go/internal/` -> `internal/`，随后按 DDD 边界拆包。
- `resources/database/*.sql` -> `migrations/` 并使用有序文件名。

### 9.2 删除

- tracked：`bootstrap/`、`framework/`、根 `pom.xml` 及 Java 专属配置/测试。
- untracked/local：`doc/knowledge-implementation-plan.md`、`.github/modernize/java-upgrade/`。
- 空的 `go/`、`resources/` 目录。

### 9.3 重写或保留

- 重写 `README.md`、`AGENTS.md`、`CLAUDE.md`、`.gitignore` 和现行 `.trellis/spec/backend/`，使其只描述 Go。
- 保留 `.trellis/tasks/archive/` 历史，即使历史内容描述 Java。
- 保留与语言无关的 Trellis/Codex 工作流文件。

## 10. 中文注释设计

生产代码：

- 每个包使用 package comment 说明职责和边界。
- 每个导出类型、接口、函数说明用途、输入输出和重要约束。
- 业务用例按执行顺序解释关键判断、状态变化、事务外慢操作和失败补偿。
- 对 `defer`、错误包装、context、接口隐式实现、事务回滚、向量 cast 等不直观写法给出大白话说明。
- 不给 `i++`、简单赋值或明显 getter 添加机械翻译式注释。

测试代码：

- 说明测试场景和风险；
- 标出 Arrange / Act / Assert 的意图；
- 解释关键 fake、失败注入和断言；
- 不逐行翻译无业务含义的测试样板。

## 11. 测试与质量策略

- 迁移前先运行现有 Go 测试，建立行为基线。
- 领域层新增状态转换和 metadata 保留测试。
- 应用层保留并迁移上传去重、存储补偿、分块成功和失败回写测试。
- PostgreSQL 适配器重点测试事务边界、维度校验、唯一冲突映射和行模型转换。
- HTTP 适配器测试响应外壳、错误映射、请求 ID 和日志字段。
- 配置测试覆盖默认值、非法 duration、缺少必要密钥以及本地存储模式。
- 生命周期测试覆盖启动失败和有期限的 Shutdown；不在单元测试中监听固定端口。

必过命令从仓库根目录执行：

```powershell
go fmt ./...
go test ./...
go vet ./...
go build ./cmd/travel-agent
git diff --check
```

另外执行仓库级检查，排除 `.git/` 和 `.trellis/tasks/archive/` 后，不得再出现 `.java`、`pom.xml`、Maven/Spring 当前运行说明或旧 `go/` 导入路径。

## 12. 迁移、回滚与风险

### 12.1 实施顺序

先建立根 Go 模块并迁移代码，通过测试后再删除 Java。这样任一中间阶段都能判断失败来自包移动还是业务改造。

### 12.2 回滚

- 本任务不执行数据库迁移，因此代码回滚不需要数据库回滚。
- 文件移动和 Java 删除都保留在 Git diff 中，审核前可以逐项恢复。
- 如果 DDD 拆包导致行为回归，优先回退对应包移动，而不是修改 API 或 schema 规避测试。

### 12.3 主要风险

- 包移动造成导入遗漏：通过 `go test ./...`、`go vet ./...` 和全仓搜索验证。
- 领域模型与数据库模型分离造成字段丢失：为双向转换建立完整测试。
- HTTP Server 写超时过短影响同步分块：默认使用 `5m` 且允许环境变量覆盖。
- 过量注释降低可读性：以“解释意图、约束和失败后果”为标准，不机械翻译语法。
- 删除未跟踪本地文件不可由 Git 恢复：用户已明确批准删除，实施时仍在删除前核对绝对路径。

## 13. 决策摘要

- 仓库最终只保留 Go 实现。
- Go 模块提升到仓库根目录。
- 使用模块化单体和轻量 DDD，`knowledge` 是当前限界上下文。
- 接口定义在使用它们的 `application` 包中，不创建独立 `port/`。
- 使用手工依赖注入，不引入 DI 框架。
- 补齐 `slog`、请求日志、请求 ID、HTTP 超时、配置校验和优雅停机。
- 保持 API、PostgreSQL schema、pgvector 1536 维和对象存储行为兼容。
- 不引入 CQRS、事件溯源、事件总线或无实际职责的目录。
