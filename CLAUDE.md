# CLAUDE.md

本文件说明在 TravelAgent 仓库中进行开发时必须遵守的当前约束。更完整的执行规则见 `AGENTS.md` 和 `.trellis/spec/backend/`。

## 项目概览

TravelAgent 是一个 Go 1.26 单服务项目，当前实现旅行知识文档的上传、内容去重、显式分块、Embedding 和 PostgreSQL/pgvector 持久化。服务默认监听 `8081`，对象存储支持 S3/RustFS 和本地目录两种模式。

## 常用命令

所有命令从仓库根目录执行：

```powershell
go test ./...
go vet ./...
go build -o .trellis/workspace/bin/travel-agent.exe ./cmd/travel-agent
go run ./cmd/travel-agent
```

`.env.example` 不会自动加载。运行前通过终端、IDE 或部署环境注入 `POSTGRESQL_DSN`、`EMBEDDING_API_KEY` 以及所选对象存储需要的配置。

## 架构边界

```text
cmd -> internal/app -> 具体 adapter/platform
                         |
HTTP adapter -> application <- PostgreSQL/Storage/Embedding
                      |
                    domain
```

- `internal/knowledge/domain` 只依赖标准库并拥有文档状态规则。
- `internal/knowledge/application` 拥有用例和外部能力小接口。
- `internal/knowledge/adapter` 处理框架、协议、SQL 和模型转换。
- `internal/platform` 提供进程级通用基础设施。
- `internal/app` 是唯一组合根，使用构造器手工注入，不使用服务定位或全局数据库。

## 数据和运行契约

- 保持 `rag` schema 和 `vector(1536)`。
- 保持六条 `/api/knowledge/...` 路由及 `code/message/data` 响应外壳。
- 上传成功只创建 `pending` 文档；分块必须显式触发。
- 解析、分块和 Embedding 位于事务外；完整结果才进入替换事务。
- 数据库创建失败时尽力补偿已上传对象；补偿错误不能覆盖原始错误。
- 请求日志使用 `slog`，包含 `request_id/method/path/status/latency_ms`，且不得记录密钥或 DSN。
- 收到 `SIGINT/SIGTERM` 后按配置超时执行优雅停机。

## 编码和测试

- 行为修改遵循红灯、最小实现、绿灯、重构的顺序。
- 错误用 `%w` 包装，分类用 `errors.Is/As`，`context.Context` 放首参并向下传递。
- 生产代码写准确、详细、通俗的中文注释；测试解释场景、准备和关键断言。
- 完成前运行 `go fmt ./...`、`go test ./...`、`go vet ./...`、构建和 `git diff --check`。

## 重要文件

- `README.md`：运行、配置、API 和 MVP 边界。
- `.env.example`：完整且不含真实凭据的环境变量模板。
- `migrations/000001_rag_baseline.sql`：只用于全新空数据库。
- `migrations/000002_knowledge_ingestion_upgrade.sql`：已有 schema 的非破坏性检查/升级脚本。
- `.trellis/tasks/07-13-go-enterprise-structure-comments/`：当前重构需求、设计和实施清单。
