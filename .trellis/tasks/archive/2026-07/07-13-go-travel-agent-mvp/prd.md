# Go 版 TravelAgent MVP

## Goal

在保留现有 Java Spring Boot 项目的前提下，新增一个 Go 版 TravelAgent MVP 服务。

Go 版 MVP 先参考 Java 版已经跑通的知识库文档摄取能力，不追求一次性替换全部 Java 功能。目标是让 Go 服务具备可独立启动、可连接现有 PostgreSQL/pgvector、可复刻核心接口、可被后续继续扩展 Agent/RAG 编排的基础。

本任务同时重写项目 README，让 README 准确说明当前仓库结构、Java 版状态、Go 版 MVP 规划、启动方式、环境变量和数据库要求。

## Background

- 当前仓库是 Maven 多模块 Java 项目，实际模块为 `framework` 和 `bootstrap`。
- 当前 README 只写了简短项目名和一个不完全准确的目录树，其中 `infra`、`mcp` 目录当前不存在。
- Java 版 `bootstrap` 是 Spring Boot 业务启动模块，`framework` 放基础设施能力。
- Java 版已经有知识库文档摄取 MVP：上传文档、显式调用分块、查询文档状态、分页查询文档、删除文档。
- Java 版接口路径集中在 `/api/knowledge`，包括：
  - `POST /api/knowledge/bases/{kb-id}/documents/upload`
  - `POST /api/knowledge/documents/{doc-id}/chunk`
  - `GET /api/knowledge/documents/{doc-id}`
  - `GET /api/knowledge/documents/{doc-id}/status`
  - `GET /api/knowledge/bases/{kb-id}/documents`
  - `DELETE /api/knowledge/documents/{doc-id}`
- 数据库使用 PostgreSQL schema `rag`，表包括：
  - `rag.t_knowledge_base`
  - `rag.t_knowledge_document`
  - `rag.t_knowledge_chunk`
  - `rag.t_knowledge_vector`
- 向量表使用 `vector(1536)` 和 HNSW cosine 索引。
- 文档表已有同知识库内容哈希去重索引：`uk_knowledge_document_kb_hash_active`。
- Java 版配置中 Embedding 模型维度是 1536，默认文档上传大小 50MB。

## Requirements

1. 保留 Java 项目，不删除、不重命名现有 `framework`、`bootstrap`、`resources` 结构。
2. 新增 Go 版 MVP 代码应作为并行实现存在，不能破坏 Java 版 Maven 构建。
3. Go 版 MVP 代码放在仓库根目录 `go/` 下，隔离 Java Maven 项目和 Go 服务代码。
4. Go 版 MVP 优先复刻 Java 版知识库文档摄取主流程：
   - 文档上传
   - 同知识库内容哈希去重
   - 文档状态查询
   - 显式调用分块接口
   - 分块后写入 chunk 表和 vector 表
   - 最近一次失败原因写入文档 metadata
5. Go 版 MVP 应复用现有 PostgreSQL/pgvector 表结构，除非规划阶段确认必须新增迁移脚本。
6. Go 版 MVP 的向量维度必须保持 1536，避免和现有 `rag.t_knowledge_vector.embedding vector(1536)` 不兼容。
7. Go Web 服务应提供 HTTP API，接口语义优先兼容 Java 版，方便后续前端或调用方替换。
8. Go Agent/RAG 编排优先按 Go 原生方案规划，初步技术倾向：
   - Web 框架：Gin
   - Agent/RAG 编排：CloudWeGo Eino
   - 数据库访问：sqlx + pgx driver
9. Go 版 MVP 生产运行需要调用真实 Embedding 模型并写入 1536 维向量；测试中使用 fake/mock Embedding，避免测试依赖外部 API。
10. README 需要重写为当前项目真实说明，至少包含：
   - 项目定位
   - 当前 Java 模块说明
   - Go MVP 模块说明
   - 技术栈
   - 数据库要求
   - 环境变量
   - Java 启动与测试命令
   - Go 启动与测试命令
   - 当前 MVP 范围与后续计划
11. 本任务先进入 Trellis 规划流程。实现前必须完成并审阅 `prd.md`、`design.md`、`implement.md`。

## Decisions

- Go 版 MVP 代码目录使用 `go/`，在其下再使用 Go 常见布局，例如 `cmd/travel-agent/`、`internal/`、`configs/`。
- Embedding 采用“生产真实调用、测试 mock”的策略：生产环境调用真实 Embedding API，测试环境固定返回 1536 维 fake 向量。
- Go 版 HTTP API 复用 Java 版 `/api/knowledge/...` 路径，通过独立端口和 Java 服务区分。建议默认 Java 运行在 `8080`，Go MVP 运行在 `8081`。
- README 写成“项目介绍 + 开发者运行手册”，既说明项目是什么，也说明 Java 和 Go 两套服务如何启动、测试和连接数据库。
- Go MVP 数据库访问采用 `sqlx + pgx driver`。MVP 阶段 SQL 贴近现有 Java mapper 和数据库表，后续表结构稳定后再评估是否引入 `sqlc` 生成强类型查询代码。

## Out of Scope

- 不在第一版 Go MVP 中删除 Java 代码。
- 不在第一版 Go MVP 中实现完整用户权限体系。
- 不在第一版 Go MVP 中新增复杂执行记录表。
- 不在第一版 Go MVP 中实现多 Agent、可视化工作流或后台任务平台。
- 不在第一版 Go MVP 中重做前端页面。

## Acceptance Criteria

- [ ] Trellis 任务包含完整 `prd.md`、`design.md`、`implement.md`。
- [ ] 规划明确 Go 代码目录位置、模块边界、接口兼容策略、数据库访问方案和验证方式。
- [ ] Go MVP 保留 Java 项目，并且 Java Maven 构建路径不被破坏。
- [ ] Go MVP 能独立启动 HTTP 服务。
- [ ] Go MVP 至少覆盖知识库文档摄取主流程的核心接口。
- [ ] Go MVP 能连接 PostgreSQL 并写入现有 `rag` schema。
- [ ] Go MVP 对 pgvector 写入使用 1536 维向量。
- [ ] Go MVP 有基础测试或可重复验证命令。
- [ ] README 被重写，内容与仓库真实结构一致。
- [ ] 最终提交前通过必要验证，至少包括 Java 现有测试和 Go 侧测试/构建。

## Open Questions

无。
