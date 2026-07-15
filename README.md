# TravelAgent

TravelAgent 是一个纯 Go 的旅行知识库服务。当前版本聚焦知识文档摄取：上传文件、按内容去重、显式分块、调用 OpenAI 兼容的 Embedding API，并把文档、分块和 1536 维向量写入 PostgreSQL/pgvector。

项目采用“Go 单服务仓库 + 模块化单体 + 轻量 DDD”。所有命令都从仓库根目录执行，代码、构建和运行入口都属于同一个根 Go 模块。

## 项目结构

```text
TravelAgent/
├── cmd/travel-agent/                  # 可执行程序入口：信号、退出码
├── internal/app/                      # 唯一组合根和 HTTP 生命周期
├── internal/platform/                 # 配置、数据库、日志中间件、存储、Embedding
├── internal/knowledge/
│   ├── domain/                        # Document 聚合、状态规则、Chunk、领域错误
│   ├── application/                   # 上传、处理、查询、删除用例及使用方接口
│   └── adapter/
│       ├── http/                      # Gin 路由、请求/响应 DTO、错误映射
│       └── postgres/                  # sqlx SQL、行模型、pgvector 和事务
├── migrations/                        # 空库基线与非破坏性升级 SQL
├── .env.example                       # 环境变量模板，不会被程序自动加载
├── go.mod
└── go.sum
```

依赖方向固定为：

```text
cmd -> app -> 具体 adapter/platform
              |
HTTP adapter -> application <- PostgreSQL/Storage/Embedding
                    |
                  domain
```

- `domain` 只依赖 Go 标准库。
- `application` 在使用方定义小接口，不依赖 Gin、sqlx、AWS SDK。
- `adapter` 负责 HTTP/数据库模型与领域对象之间的显式转换。
- `internal/app` 使用构造器手工注入所有长期依赖，不使用 DI 容器，也不把服务单例塞进 `gin.Context`。

## 技术栈与运行要求

- Go 1.26+
- Gin 1.11
- sqlx + pgx v5
- PostgreSQL 16+ 与 pgvector
- AWS SDK for Go v2（Amazon S3/RustFS 兼容存储）
- OpenAI-compatible Embeddings API
- 标准库 `log/slog`、`net/http`、`os/signal`

PostgreSQL 使用 `rag` schema，核心表为：

```text
rag.t_knowledge_base
rag.t_knowledge_document
rag.t_knowledge_chunk
rag.t_knowledge_vector
```

向量字段必须保持：

```sql
embedding vector(1536) NOT NULL
```

## 数据库 SQL

- `migrations/000001_rag_baseline.sql`：只用于全新空数据库初始化，包含重建语句，禁止对已有业务数据的数据库直接执行。
- `migrations/000002_knowledge_ingestion_upgrade.sql`：用于检查和补齐文档摄取相关约束；执行前仍应备份并由数据库负责人审核。

应用启动不会自动执行这两份 SQL，也不会自动删除或重建现有数据。

## 环境变量

`.env.example` 只是模板，Go 进程不会自动读取 `.env`。请由 PowerShell、容器、IDE Run Configuration 或部署平台注入环境变量。

最小本地示例（使用本地文件存储）：

```powershell
$env:POSTGRESQL_DSN='postgres://user:password@localhost:5432/kenagent?sslmode=disable'
$env:EMBEDDING_API_KEY='replace-me'
$env:RUSTFS_ENABLED='false'
$env:LOCAL_STORAGE_DIR='.data/storage'
go run ./cmd/travel-agent
```

主要配置如下：

| 环境变量 | 默认值 | 说明 |
|---|---:|---|
| `GO_AGENT_PORT` | `8081` | HTTP 监听端口 |
| `HTTP_READ_HEADER_TIMEOUT` | `5s` | 读取请求头上限 |
| `HTTP_READ_TIMEOUT` | `60s` | 读取完整请求上限 |
| `HTTP_WRITE_TIMEOUT` | `5m` | 写响应上限，为同步文档处理预留时间 |
| `HTTP_IDLE_TIMEOUT` | `60s` | Keep-Alive 空闲上限 |
| `HTTP_SHUTDOWN_TIMEOUT` | `15s` | 优雅停机等待上限 |
| `POSTGRESQL_DSN` | 无，必填 | PostgreSQL DSN；不得写入日志或提交真实密码 |
| `POSTGRESQL_MAX_OPEN_CONNS` | `10` | 最大打开连接数 |
| `POSTGRESQL_MAX_IDLE_CONNS` | `5` | 最大空闲连接数 |
| `POSTGRESQL_CONN_MAX_LIFETIME` | `30m` | 单连接最长生命周期 |
| `POSTGRESQL_CONN_MAX_IDLE_TIME` | `5m` | 单连接最长空闲时间 |
| `RUSTFS_ENABLED` | `true` | `true` 使用 S3/RustFS，`false` 使用本地目录 |
| `RUSTFS_BUCKET_NAME` | 无 | 启用 S3/RustFS 时必填 |
| `RUSTFS_ENDPOINT` | `http://localhost:9000` | S3/RustFS endpoint |
| `RUSTFS_REGION` | `us-east-1` | S3 region |
| `RUSTFS_ACCESS_KEY` | 无 | 启用 S3/RustFS 时必填 |
| `RUSTFS_SECRET_KEY` | 无 | 启用 S3/RustFS 时必填 |
| `RUSTFS_PATH_STYLE` | `true` | RustFS 常用 path-style 请求 |
| `LOCAL_STORAGE_DIR` | `.data/storage` | 本地存储根目录 |
| `EMBEDDING_API_KEY` | 无，必填 | Embedding API 密钥 |
| `EMBEDDING_BASE_URL` | `https://dashscope.aliyuncs.com/compatible-mode` | OpenAI 兼容 API 根地址 |
| `EMBEDDING_MODEL` | `text-embedding-v3` | Embedding 模型 |
| `EMBEDDING_DIMENSIONS` | `1536` | 必须与 pgvector 列一致 |
| `EMBEDDING_TIMEOUT` | `60s` | 单次 Embedding HTTP 请求上限 |
| `KNOWLEDGE_DOCUMENT_ALLOWED_EXTENSIONS` | `pdf,doc,docx,txt,md,markdown,html,htm` | 上传扩展名白名单 |
| `KNOWLEDGE_DOCUMENT_MAX_SIZE` | `50MB` | 单文件最大值 |
| `LOG_LEVEL` | `info` | `debug/info/warn/error` |
| `LOG_FORMAT` | `json` | `json/text` |

## 构建、测试和启动

以下命令全部从仓库根目录执行：

```powershell
go test ./...
go vet ./...
go build -o .trellis/workspace/bin/travel-agent.exe ./cmd/travel-agent
go run ./cmd/travel-agent
```

Windows 上如果默认 Go 缓存目录不可写，可把缓存放到仓库本地（这些目录已被忽略）：

```powershell
$env:GOTELEMETRY='off'
$env:GOTELEMETRYDIR="$PWD\.cache\telemetry"
$env:GOCACHE="$PWD\.cache\go-build"
$env:GOMODCACHE="$PWD\.cache\go-mod"
go test ./...
```

服务默认地址为 `http://localhost:8081`。收到 Ctrl+C、`SIGINT` 或 `SIGTERM` 后，服务停止接收新请求，并在 `HTTP_SHUTDOWN_TIMEOUT` 内等待正在处理的请求结束。

## HTTP 接口

```text
GET    /health
POST   /api/knowledge/bases/{kb-id}/documents/upload
POST   /api/knowledge/documents/{doc-id}/chunk
GET    /api/knowledge/documents/{doc-id}
GET    /api/knowledge/documents/{doc-id}/status
GET    /api/knowledge/bases/{kb-id}/documents
DELETE /api/knowledge/documents/{doc-id}
```

知识接口统一使用：

```json
{
  "code": "0",
  "message": "",
  "data": {}
}
```

请求中间件会生成或沿用 `X-Request-ID`，并用 `slog` 记录 `request_id`、方法、路径、状态码和耗时。

## 文档处理流程

1. 上传入口校验知识库、文件大小和扩展名。
2. 读取受限字节数并计算 `SHA-256`，同知识库按内容哈希去重。
3. 把文件写入 S3/RustFS 或本地存储，再创建 `pending` 文档。
4. 数据库创建失败时尽力删除已写对象，但补偿失败不会覆盖原始数据库错误。
5. 调用分块接口后，用条件更新原子抢占 `processing` 状态。
6. 在数据库事务外读取、解析、分块、调用 Embedding，并校验每个向量恰好 1536 维。
7. 在一个短事务内删除旧向量/分块、写入新分块/向量并把文档更新为 `completed`。
8. 处理失败时保存 `failed` 和 `metadata.lastError`，同时保留其他 metadata。

当前可解析并分块的格式是 `txt`、`md`、`markdown`、`html`、`htm`。`pdf`、`doc`、`docx` 可以通过上传白名单，但处理阶段会返回明确的暂不支持错误；完整二进制文档解析、权限体系、多 Agent 编排、可视化工作流和前端页面不在当前 MVP 范围内。
