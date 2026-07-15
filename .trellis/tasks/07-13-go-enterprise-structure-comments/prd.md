# 规范化 Go 项目结构并补充中文注释

## Goal

移除仓库中的 Java 实现及 Java 专属工程配置，把 TravelAgent 收敛为纯 Go 项目，并在不改变现有 Go MVP 业务行为的前提下，将项目整理为职责清楚、依赖方向稳定、便于测试和继续扩展的企业级工程结构。同时为 Go 代码补充详细、面向初学者的中文大白话注释，让读者不仅知道代码“做了什么”，也能理解“为什么这样写、数据如何流动、失败时会怎样”。

## Background

- Go 版 MVP 已于提交 `419df68` 合入 `main`，上一 Trellis 任务 `07-13-go-travel-agent-mvp` 已归档。
- 当前仓库仍是 Java Maven 多模块项目与 `go/` 子模块并存；用户已明确允许删除 Java 相关内容，最终仓库只保留 Go 实现。
- 当前 Go 模块包含 17 个 `.go` 文件，其中 12 个生产代码文件、5 个测试文件。
- 改造前已在现有 `go/` 子模块运行 `go test ./...` 建立基线；现有测试全部通过，无需先修复历史业务失败。
- 当前生产代码分布在：
  - `cmd/travel-agent`：启动入口；
  - `internal/config`：环境变量与配置；
  - `internal/database`：数据库连接；
  - `internal/http`：Gin 路由、处理器和统一响应；
  - `internal/knowledge`：知识文档领域类型、业务服务、分块算法和 SQL 仓储；
  - `internal/embedding`：真实与测试向量模型实现；
  - `internal/storage`：本地与 S3/RustFS 存储实现。
- 现有 Go API、PostgreSQL `rag` schema、1536 维向量及当前对外响应语义需要保持不变。

## Requirements

1. 删除 Java 源码、Maven 模块、Java 构建文件及只服务于 Java 实现的配置、文档和测试；删除前必须逐项盘点，不能误删 Go 运行仍复用的 SQL、数据库约束、文档或项目级工具配置。
2. 最终仓库应成为可直接按 Go 工程理解、构建和运行的纯 Go 项目；目录和包边界应体现企业级职责划分，并遵守 Go 的 `internal` 可见性习惯。
3. 采用 Go 单服务仓库的标准根目录布局：把当前 `go/go.mod`、`go/go.sum`、`go/cmd/`、`go/internal/` 和 `go/.env.example` 提升到仓库根目录，迁移完成后删除空的 `go/` 外壳。
4. `resources/database/` 中仍被 Go 使用的 PostgreSQL/pgvector SQL 不删除，整理到根目录 `migrations/`。
5. 所有 Go 构建、测试和启动命令从仓库根目录直接执行，不再要求先进入 `go/` 子目录。
6. 根模块路径根据当前 Git 远端统一为 `github.com/luqingjia/TravelAgent`，内部导入同步使用该模块路径。
7. 依赖方向必须明确，业务核心不能反向依赖 Gin、SQL、S3 等具体基础设施实现。
8. 企业级结构采用“按业务模块组织的模块化单体 + 轻量 DDD”：
   - `internal/app` 负责依赖组装、服务启动和关闭；
   - `internal/platform` 放置配置、数据库连接、Embedding 和对象存储等通用基础设施；
   - `knowledge` 是当前限界上下文，`internal/knowledge/domain` 放置 `Document` 聚合、`Chunk`、状态、业务错误和状态转换规则；
   - `internal/knowledge/application` 放置上传、分块、查询和删除等业务用例，并在使用方定义仓储、文件存储和向量模型等小接口；
   - `internal/knowledge/adapter/http` 与 `internal/knowledge/adapter/postgres` 分别实现 Gin 接入和 PostgreSQL 持久化。
   - HTTP DTO、数据库行模型与领域对象分离，适配层负责转换；领域层不能出现 `json`、`db`、Gin、sqlx 或 AWS SDK 耦合。
   当前没有实际业务的未来模块不得提前创建空目录或占位接口；不引入当前业务不需要的 CQRS、事件溯源、通用仓储基类或事件总线。
9. 补齐最小生产运行保障：
   - 使用标准库 `log/slog` 输出结构化日志；
   - HTTP 中间件记录请求 ID、方法、路径、状态码和耗时；
   - `http.Server` 配置读取、写入和空闲超时；
   - 监听 `SIGINT`、`SIGTERM`，停止接收新请求并在限定时间内优雅关闭；
   - 启动时集中校验数据库、Embedding、存储等必要配置；
   - 保留 Gin panic recovery，但不在本任务引入 Prometheus、OpenTelemetry 或外部日志平台。
10. 依赖注入采用 Go 构造器手工注入：所有长期依赖在 `internal/app` 组合根显式创建并传给构造函数，不引入运行时 DI 容器或代码生成 DI 框架。Gin Context 仅保存请求 ID、认证主体等请求级数据，不存放数据库、仓储、应用服务或客户端单例。
11. 保持现有 Go HTTP 路径、响应语义、数据库表结构、对象存储行为和 Embedding 维度兼容，不再把 Java 构建或 Java 接口实现作为最终验收依赖。
12. 重构时优先复用现有实现，不为了目录“看起来完整”而引入当前业务不需要的空层、空包或复杂框架。
13. 给 Go 代码增加详细中文大白话注释，重点说明：
   - 包和文件负责什么；
   - 结构体、接口和关键字段为什么存在；
   - 关键函数的输入、输出、处理步骤和错误路径；
   - 事务边界、状态转换、资源补偿、向量维度等容易踩坑的设计；
   - Go 语法或标准库写法中对初学者不直观的部分。
14. 注释覆盖规则确定为：
   - 所有生产代码：为包、结构体、接口、函数、关键步骤和难懂语句补充详细中文大白话说明；
   - 业务代码：沿真实执行顺序，对关键业务语句进行细粒度、接近逐行的中文解释，明确每一步的数据变化、判断原因和失败后果；
   - 测试代码：重点解释测试场景、测试数据准备、被验证的行为和关键断言，不机械翻译每一条测试辅助语句。
15. 注释必须与真实代码一致，不能只复述变量名或语法，也不能用注释掩盖职责过重、命名不清等结构问题。
16. 重写根目录 README、启动命令、构建说明和环境变量文档，使其只描述真实存在的 Go 项目。
17. 清理或改写仍引用 Maven、Spring Boot、Java 模块路径的项目级脚本、CI 配置和规范文档；Trellis、Codex 等开发工作流文件仅在确实与语言无关或已同步为 Go 规范时保留。
18. 删除未被 Git 跟踪但只服务于 Java 的本地资料：`doc/knowledge-implementation-plan.md` 和 `.github/modernize/java-upgrade/`。
19. 保留 `.trellis/tasks/archive/` 中的历史任务记录；历史记录允许描述过去的 Java 实现，但不得被 README、构建脚本或现行规范当作当前架构。
20. 重构后补齐或调整测试，确保原有 Go 行为保持一致。
21. 本任务属于复杂改造，实施前必须完成并由用户审核 `prd.md`、`design.md`、`implement.md`。

## Acceptance Criteria

- [ ] 纯 Go 仓库的根目录结构、包职责和依赖方向在 `design.md` 中明确。
- [ ] 现有 17 个 Go 文件均被纳入迁移或保留清单，不遗漏生产代码和测试。
- [ ] 仓库中不再保留 Java 源码、`pom.xml`、Maven 模块或只服务于 Java 的构建配置。
- [ ] Go 模块可从规划确定的项目根目录直接执行标准构建、测试和启动命令。
- [ ] 仓库根目录存在 `go.mod`、`go.sum`、`cmd/`、`internal/`、`.env.example` 和 `migrations/`，不再存在仅用于包裹 Go 模块的 `go/` 子目录。
- [ ] `go.mod` 模块路径为 `github.com/luqingjia/TravelAgent`，仓库内导入路径一致。
- [ ] 重构后 Go 服务可构建、可启动，现有核心 HTTP 接口保持兼容。
- [ ] 数据库 schema、SQL 语义、文档状态流转和 1536 维向量约束保持兼容。
- [ ] 业务核心通过接口依赖基础设施能力，Gin、SQL、S3 等实现位于合适的适配层。
- [ ] `knowledge` 限界上下文按 `domain / application / adapter` 划分，应用层在使用方定义小接口，公共运行基础设施放入 `platform`，且没有无实际职责的占位包。
- [ ] `Document` 聚合封装 `pending / processing / completed / failed` 状态转换及完成、失败时的元数据规则，HTTP/数据库模型不直接充当领域模型。
- [ ] 服务使用 `slog` 结构化日志，请求日志包含请求 ID、方法、路径、状态码和耗时。
- [ ] HTTP 服务配置合理超时，并能在收到 `SIGINT/SIGTERM` 时进行有期限的优雅关闭。
- [ ] 必要配置在建立外部连接和启动监听前完成校验，错误信息能够指出具体配置项。
- [ ] 数据库、仓储、存储、Embedding、应用服务和 Handler 均通过构造函数在 `internal/app` 显式组装；项目不包含 DI 容器，Gin Context 不承担服务定位职责。
- [ ] 所有生产代码均包含准确、详细、通俗的中文注释，业务代码的关键执行语句具有按执行顺序展开的大白话说明。
- [ ] 测试代码均说明测试场景、准备过程和关键断言，不要求对无业务含义的辅助语句逐行翻译。
- [ ] `go test ./...` 和 `go build ./cmd/travel-agent` 通过。
- [ ] README、环境变量示例、构建命令及相关项目说明只描述最终纯 Go 项目。
- [ ] 项目级脚本、CI 和 Trellis 后端规范中不存在失效的 Java/Maven 路径引用。
- [ ] `doc/knowledge-implementation-plan.md` 和 `.github/modernize/java-upgrade/` 已删除，Trellis 归档历史仍保留。
- [ ] `git diff --check` 通过，且没有提交密钥、运行缓存或生成物。

## Out of Scope

- 不新增与本次结构规范化无关的业务功能。
- 不替换现有数据库表或修改向量维度。
- 不为了套用所谓“企业模板”而新增没有实际职责的包。
- 不引入 CQRS、事件溯源、领域事件总线、通用仓储基类或为每张数据库表建立独立聚合。
- 不删除或重建现有 PostgreSQL 业务数据；本任务只处理代码仓库及必要的兼容迁移说明。
