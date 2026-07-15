# Repository Guidelines

## Project Structure

TravelAgent 是一个从仓库根目录构建和运行的 Go 单服务项目。

- `cmd/travel-agent/`：进程入口，只处理信号、调用 `app.Run` 和退出码。
- `internal/app/`：唯一组合根，创建具体依赖并管理 HTTP/数据库生命周期。
- `internal/knowledge/domain/`：知识文档聚合、状态转换、分块值对象和领域错误；只依赖标准库。
- `internal/knowledge/application/`：上传、处理、查询、删除用例，以及由使用方定义的仓储、存储、Embedding 小接口。
- `internal/knowledge/adapter/http/`：Gin 路由、请求/响应 DTO、错误映射。
- `internal/knowledge/adapter/postgres/`：sqlx 行模型、SQL、pgvector 转换和替换事务。
- `internal/platform/`：配置、数据库连接、HTTP 中间件、对象存储、Embedding 客户端。
- `migrations/`：人工审核和执行的数据库 SQL；应用不得自动运行。

不要提前创建没有真实业务的空模块，也不要新增 `common`、`utils`、`models` 这类职责不清的兜底包。

## Dependency Boundaries

- `domain` 只能导入 Go 标准库。
- `application` 可以导入 `domain`，但不能导入 Gin、sqlx、pgx、AWS SDK 或具体 platform 实现。
- HTTP 适配器不能直接访问数据库、对象存储或 Embedding 具体实现。
- PostgreSQL、存储和 Embedding 适配器向内实现 application 定义的小接口。
- 只有 `internal/app` 可以同时导入具体适配器并组装完整对象图。
- 依赖使用构造函数手工注入；不要引入 DI 容器，不要用全局变量保存数据库或服务。
- `gin.Context` 只放请求 ID、认证主体、trace 等请求级数据，不承担服务定位。

## Build, Test, and Run

所有命令从仓库根目录执行：

```powershell
go test ./...
go vet ./...
go build -o .trellis/workspace/bin/travel-agent.exe ./cmd/travel-agent
go run ./cmd/travel-agent
```

`.env.example` 只是模板，程序不会自动加载 `.env`。本地运行时通过 PowerShell、IDE 或容器显式注入环境变量。

## Go Conventions

- 使用 `gofmt`；包名短小、小写，导出标识符使用 `PascalCase`，内部标识符使用 `camelCase`。
- 接收 `context.Context` 的函数把它放在第一个参数，并把调用方 context 继续传到数据库、HTTP 和存储操作。
- 错误用 `fmt.Errorf("operation: %w", err)` 增加操作上下文；分类使用 `errors.Is/As`，不要比较错误字符串。
- 构造函数校验长期依赖和稳定配置，发现缺失立即返回错误，不要把失败推迟到第一次请求。
- 外部 DTO、数据库行模型和领域对象必须分离，并在适配器边界显式转换。
- pgvector 固定为 1536 维；写入 SQL 使用显式 `::vector`，不能让驱动猜测 PostgreSQL 专属类型。
- 文档分块的慢操作在事务外执行；替换旧分块、旧向量、新数据和完成状态必须处于同一个短事务。

## Commenting Requirements

- 每个生产包写 package comment，说明职责和不能做什么。
- 生产代码为结构体、接口、函数、关键步骤和难懂语句写准确、通俗的中文注释。
- 业务用例按真实执行顺序解释校验、数据变化、状态转换、外部调用、事务边界和失败补偿。
- 测试代码说明测试场景、准备过程、故障注入和关键断言，不机械翻译无业务含义的样板语句。
- 注释解释“为什么”和失败后果，不要写“给变量赋值”“进入 if”之类语法复述。

## Testing Guidelines

- 行为变更先写失败测试，确认失败原因正确，再写最小实现并跑回归。
- 领域测试覆盖状态转换和不变量；应用测试使用 fake 端口覆盖业务编排和补偿；适配器测试覆盖边界转换、SQL/向量格式和 HTTP 兼容性。
- 测试不得依赖真实云凭据、固定开发端口或生产数据库。
- 完成前至少通过 `go fmt ./...`、`go test ./...`、`go vet ./...`、`go build ./cmd/travel-agent` 和 `git diff --check`。

## Tool-assisted Discovery

- 查询 Gin、sqlx、pgx、AWS SDK 等库的当前 API 时，先使用 Context7 获取对应版本的官方文档。
- 查找代码符号、调用关系和依赖影响时，优先使用 codebase-memory 图谱；只有图谱不足或查找非代码文本时才使用 `rg`。

## Security and Configuration

- 不提交 API key、数据库密码、对象存储密钥、`.env`、本地数据、缓存或构建产物。
- 日志不得输出 DSN、Authorization header、API key、access key、secret key 或完整上传内容。
- `migrations/000001_rag_baseline.sql` 只用于全新空库；任何 SQL 都必须人工审核，应用启动不得自动执行。

## Commit and Review

提交信息保持简短，例如 `refactor(go): standardize DDD project structure`。提交或 PR 说明应列出验证命令，并明确数据库、pgvector、环境变量或迁移要求。

<!-- TRELLIS:START -->
# Trellis Instructions

These instructions are for AI assistants working in this project.

This project is managed by Trellis. The working knowledge you need lives under `.trellis/`:

- `.trellis/workflow.md` — development phases, when to create tasks, skill routing
- `.trellis/spec/` — package- and layer-scoped coding guidelines (read before writing code in a given layer)
- `.trellis/workspace/` — per-developer journals and session traces
- `.trellis/tasks/` — active and archived tasks (PRDs, research, jsonl context)

If a Trellis command is available on your platform (e.g. `/trellis:finish-work`, `/trellis:continue`), prefer it over manual steps. Not every platform exposes every command.

If you're using Codex or another agent-capable tool, additional project-scoped helpers may live in:
- `.agents/skills/` — reusable Trellis skills
- `.codex/agents/` — optional custom subagents

Managed by Trellis. Edits outside this block are preserved; edits inside may be overwritten by a future `trellis update`.

<!-- TRELLIS:END -->
