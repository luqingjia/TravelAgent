# TravelAgent

TravelAgent 是一个面向旅行知识库、RAG 和 Agent 能力的后端项目。

当前仓库保留已有 Java Spring Boot 实现，并新增 Go 版 MVP 作为并行服务。Go 版不会替换 Java 版，第一阶段目标是复刻知识库文档摄取主流程，验证 Go 服务能独立启动、连接 PostgreSQL/pgvector、完成文档上传、显式分块和向量入库。

## 当前架构

```text
TravelAgent/
├── bootstrap/     # Java 业务启动模块，包含 controller、service、mapper、entity、测试
├── framework/     # Java 基础框架模块，包含响应封装、异常、存储等基础能力
├── go/            # Go 版 TravelAgent MVP，并行实现
├── resources/     # 数据库 SQL 等共享资源
├── .trellis/      # Trellis 任务、规范和工作流文件
└── pom.xml        # Java Maven 父工程
```

## 技术栈

Java 版：

- Java 21
- Spring Boot
- Spring AI
- MyBatis-Plus
- PostgreSQL
- pgvector
- RustFS/S3 兼容对象存储

Go MVP：

- Go 1.26+
- Gin
- sqlx
- pgx driver
- AWS SDK for Go v2，用于 S3/RustFS 兼容对象存储
- PostgreSQL + pgvector
- CloudWeGo Eino 作为后续 Agent/RAG 编排方向，第一版 MVP 先预留，不强制接入复杂编排

## 数据库要求

项目使用 PostgreSQL，并要求安装 pgvector 扩展。

核心 schema 和表：

```text
schema: rag

rag.t_knowledge_base
rag.t_knowledge_document
rag.t_knowledge_chunk
rag.t_knowledge_vector
```

向量字段要求：

```sql
embedding vector(1536) NOT NULL
```

初始化 SQL 位于：

```text
resources/database/rag.sql
resources/database/knowledge-ingestion-mvp-upgrade.sql
```

## Java 版运行

从仓库根目录执行：

```powershell
mvn spring-boot:run -pl bootstrap
```

测试：

```powershell
mvn -q test
```

打包：

```powershell
mvn -q clean package
```

Java 版默认服务端口来自 Spring Boot 配置，当前主要接口路径为：

```text
/api/knowledge/...
```

## Go MVP 运行

Go 代码位于：

```text
go/
├── cmd/travel-agent/
└── internal/
```

从 `go/` 目录执行：

```powershell
go test ./...
go build ./cmd/travel-agent
```

启动：

```powershell
go run ./cmd/travel-agent
```

Go MVP 默认监听：

```text
http://localhost:8081
```

Go 版复用 Java 版接口路径，通过端口区分服务：

```text
Java: http://localhost:8080/api/knowledge/...
Go:   http://localhost:8081/api/knowledge/...
```

## Go MVP 环境变量

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

如果本地暂时没有 RustFS/S3，可以设置：

```text
RUSTFS_ENABLED=false
LOCAL_STORAGE_DIR=.data/storage
```

这样 Go MVP 会使用本地文件存储作为开发 fallback。

## 知识库文档摄取流程

Go MVP 参考 Java 版流程：

1. 上传文档，保存为 `pending`。
2. 同知识库内按 `SHA-256(file bytes)` 去重。
3. 文件保存到 RustFS/S3 或本地 fallback。
4. 显式调用分块接口。
5. 文档从 `pending` 或 `failed` 原子进入 `processing`。
6. 解析文本、分块、调用 Embedding。
7. 成功后在一个事务内替换 chunks 和 vectors，并把文档标记为 `completed`。
8. 失败后把文档标记为 `failed`，最近一次错误写入 `metadata.lastError`。

第一版 Go MVP 优先支持以下格式的文本解析和分块：

```text
txt
md
markdown
html
htm
```

`pdf`、`doc`、`docx` 第一版可上传，但 Go 分块阶段会返回明确的暂不支持错误；后续再根据依赖成本补齐解析能力。

## 核心接口

```text
POST   /api/knowledge/bases/{kb-id}/documents/upload
POST   /api/knowledge/documents/{doc-id}/chunk
GET    /api/knowledge/documents/{doc-id}
GET    /api/knowledge/documents/{doc-id}/status
GET    /api/knowledge/bases/{kb-id}/documents
DELETE /api/knowledge/documents/{doc-id}
```

响应结构兼容 Java `Result<T>` 风格：

```json
{
  "code": "0",
  "message": "",
  "data": {}
}
```

## 当前 MVP 不包含

- 删除 Java 项目
- 完整用户权限体系
- 历史执行记录表
- 多 Agent 编排
- 可视化工作流
- 前端页面改造

## 后续计划

- 补齐 Go 版 PDF、DOC、DOCX 解析能力。
- 增加 pgvector 集成测试。
- 接入 CloudWeGo Eino，实现更完整的 Agent/RAG 问答流程。
- 根据 Go 版 SQL 稳定情况评估是否引入 sqlc。
- 逐步评估 Java 与 Go 的接口兼容和替换策略。
