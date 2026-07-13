# Go 版 TravelAgent MVP 技术设计

## Architecture

Go 版 MVP 作为 Java 项目的并行服务存在，代码放在仓库根目录 `go/` 下，不改动现有 Maven 模块边界。

推荐目录：

```text
go/
  cmd/travel-agent/          # Go 服务启动入口
  internal/config/           # 环境变量和配置加载
  internal/http/             # Gin 路由、handler、响应封装
  internal/knowledge/        # 知识库文档摄取业务
  internal/storage/          # S3/RustFS 对象存储适配
  internal/embedding/        # Embedding 接口、真实客户端、测试 fake
  internal/database/         # PostgreSQL 连接、事务辅助
  internal/pkg/              # 小范围通用工具，例如 ID、hash、metadata
```

Go 服务默认监听 `8081`。Java 服务继续默认监听 `8080`。Go HTTP API 复用 Java 的 `/api/knowledge/...` 路径，靠端口或网关路由区分服务。

## Framework Choices

- Web 框架使用 Gin：负责路由、multipart 上传、JSON 绑定、错误响应和中间件。
- Agent/RAG 编排预留 CloudWeGo Eino：第一版重点是文档摄取和向量入库，Eino 依赖可以在需要最小 Agent/RAG 问答接口时接入。
- 数据库访问使用 `sqlx + pgx driver`：MVP 阶段 SQL 需要贴近现有表和 Java mapper，手写 SQL 更直观，后续稳定后再评估 `sqlc`。
- pgvector 写入使用普通 SQL 参数拼接为 PostgreSQL vector 文本格式，并显式 cast 为 `::vector`；测试必须覆盖 1536 维向量写入。

## HTTP Contract

Go MVP 复用 Java 版核心路径：

```text
POST   /api/knowledge/bases/{kb-id}/documents/upload
POST   /api/knowledge/documents/{doc-id}/chunk
GET    /api/knowledge/documents/{doc-id}
GET    /api/knowledge/documents/{doc-id}/status
GET    /api/knowledge/bases/{kb-id}/documents
DELETE /api/knowledge/documents/{doc-id}
```

响应结构建议先兼容 Java 的 `Result<T>` 风格：

```json
{
  "code": "0",
  "message": "success",
  "data": {}
}
```

如果现有 Java `Result` 实际字段不同，实施阶段以 Java `framework` 中真实定义为准。

## Document Ingestion Flow

上传流程：

1. 校验知识库存在。
2. 校验文件扩展名和大小，默认允许 `pdf,doc,docx,txt,md,markdown,html,htm`，默认最大 50MB。
3. 读取文件内容并计算 SHA-256。
4. 在同一个知识库内检查未删除文档的 `content_hash` 是否重复。
5. 上传对象存储，记录 `source_uri`、文件名、文件类型和大小。
6. 保存 `rag.t_knowledge_document`，状态为 `pending`。
7. 如果数据库保存失败，补偿删除刚上传的对象。

显式分块流程：

1. 通过原子更新把文档从 `pending` 或 `failed` 标记为 `processing`，避免重复处理。
2. 从对象存储读取文件内容。
3. 解析文本。
4. 使用结构感知分块策略；MVP 可先实现文本、Markdown、HTML 的直接文本处理，复杂 Office/PDF 解析可按依赖能力逐步补齐。
5. 调用真实 Embedding API 生成 1536 维向量。
6. 在一个数据库事务中替换旧 chunks、vectors，并把文档状态更新为 `completed`。
7. 失败时把文档状态更新为 `failed`，把最近一次错误写入文档 `metadata.lastError`。

## Database Design

Go MVP 复用现有 `rag` schema，不新增表。

核心表：

- `rag.t_knowledge_base`
- `rag.t_knowledge_document`
- `rag.t_knowledge_chunk`
- `rag.t_knowledge_vector`

必须保持：

- `t_knowledge_vector.embedding` 使用 `vector(1536)`。
- 同知识库未删除文档使用 `(kb_id, content_hash)` 唯一约束防重复。
- 文档状态使用 `pending`、`processing`、`completed`、`failed`。
- 最近一次错误写入 `metadata.lastError`，不新增执行记录表。

Go 侧事务边界：

- 文件上传不放进数据库事务，因为对象存储不是同一个事务资源。
- 慢操作不放进数据库事务，包括对象存储读取、文档解析、分块、Embedding 调用。
- 替换 chunks、vectors、文档状态必须在一个事务里完成。

## Configuration

Go MVP 通过环境变量配置：

```text
GO_AGENT_PORT=8081
POSTGRESQL_DSN=postgres://user:password@localhost:5432/kenagent?sslmode=disable
RUSTFS_ENABLED=true
RUSTFS_BUCKET_NAME=
RUSTFS_ENDPOINT=http://localhost:9000
RUSTFS_REGION=us-east-1
RUSTFS_ACCESS_KEY=
RUSTFS_SECRET_KEY=
EMBEDDING_API_KEY=
EMBEDDING_BASE_URL=https://dashscope.aliyuncs.com/compatible-mode
EMBEDDING_MODEL=text-embedding-v3
EMBEDDING_DIMENSIONS=1536
KNOWLEDGE_DOCUMENT_ALLOWED_EXTENSIONS=pdf,doc,docx,txt,md,markdown,html,htm
KNOWLEDGE_DOCUMENT_MAX_SIZE=50MB
```

实施阶段可使用 `.env.example` 记录变量名，但不能提交真实密钥。

## README Rewrite

README 写成“项目介绍 + 开发者运行手册”，覆盖：

- 项目定位：TravelAgent 是面向旅行知识库和 RAG/Agent 能力的后端项目。
- 当前架构：Java 版为已存在实现，Go MVP 为并行新实现。
- 目录结构：必须以真实仓库为准，不能继续写不存在的 `infra`、`mcp`。
- 技术栈：Spring Boot、MyBatis-Plus、PostgreSQL/pgvector、Gin、Eino、sqlx。
- 数据库要求：PostgreSQL、pgvector、`rag` schema、1536 维向量。
- 环境变量：Java 和 Go 分开列。
- 启动、测试、打包命令：Java 和 Go 分开列。
- MVP 范围和暂不支持内容。

## Testing And Validation

规划中的验证命令：

```powershell
mvn -q test
mvn -q clean package
cd go
go test ./...
go build ./cmd/travel-agent
```

如果 Go 侧引入 PostgreSQL/pgvector 集成测试，应优先使用 Testcontainers 或可本地重复执行的 Docker 测试环境。外部 Embedding API 不应成为普通测试的硬依赖，测试使用 fake 1536 维向量。

## Rollout And Rollback

Go 服务是并行新增实现，不替换 Java 服务。

回滚方式：

- 停止 Go 服务即可恢复到仅 Java 服务。
- 如果只改 README 或 Go 目录，回滚不影响 Java 构建。
- 如果后续必须修改 SQL 脚本，应单独提交并明确兼容 Java 版。
