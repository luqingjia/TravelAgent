# Eino Agent 后端实施计划

## Preconditions

- 父任务合同已冻结；本任务 `prd.md` + `design.md` + 本文件审批后才 `task.py start`。
- 实施前加载 `trellis-before-dev`，读取 backend 规范。
- 不调用真实模型供应商；统一 fake / `httptest.Server`。
- 工作分支：`feature/eino-agent-chat-ui`。

## Implementation Checklist

1. **配置与失败测试**
   - 扩展 `platform/config` 解析 Agent 环境变量。
   - 先写配置校验失败测试：空模型、重复 ID、非法 URL/timeout、缺密钥、默认模型不存在/未启用、限制非法。
   - 模板只写占位密钥。

2. **domain**
   - 模型公开元数据、CatalogView、Message、限制常量/校验、错误 sentinel。
   - 测试：角色白名单、最后一条 user、字符/条数限制。

3. **application**
   - 定义 `ModelCatalog`、`AgentRuntime`、`StreamEvent`。
   - 实现模型选择、历史校验、Chat/Stream 共享编排。
   - fake runtime 覆盖默认模型、指定模型、边界错误、取消、运行错误。

4. **adapter/eino**
   - 按模型创建 OpenAI ChatModel、`ChatModelAgent`、Runner。
   - 注册 `get_current_time`。
   - 把 Runner 事件映射为框架无关 StreamEvent。
   - 本地 fake OpenAI 验证普通回复与工具路径；无效时区分类错误。

5. **adapter/http**
   - DTO 与统一 Result 外壳（可复用/抽取兼容结构，避免 knowledge 反向依赖）。
   - `RegisterRoutes`：models / chat / stream。
   - SSE headers、flush、预检 JSON 4xx、流后 error event、Context cancel。

6. **根 Router 迁移**
   - `platform/httpserver.NewRouter` 创建 Engine + 中间件 + `/health`。
   - knowledge `NewRouter` 改为 `RegisterRoutes` 或保留兼容包装。
   - app 组合根注册 knowledge + agent。
   - 兼容测试覆盖原 knowledge 路径与中间件顺序。

7. **组合根与清理**
   - app 构造 agent 依赖。
   - 后置失败关闭数据库。
   - 日志不输出密钥/完整 prompt。

8. **文档与 Docker**
   - 更新 `backend/.env.example`、`docker/env.example`、`docker/docker-compose.yml`、后端 README。
   - 说明模型配置、接口、安全边界。

9. **质量门**
   - `go fmt ./...`
   - `go test ./...`
   - `go vet ./...`
   - `go build -o ../.trellis/workspace/bin/travel-agent.exe ./cmd/travel-agent`
   - `git diff --check`
   - 派发 `trellis-check`

## Validation Commands

```powershell
cd backend
go fmt ./...
go test ./...
go vet ./...
go build -o ../.trellis/workspace/bin/travel-agent.exe ./cmd/travel-agent
cd ..
git diff --check
```

## Review Gates

| Gate | 通过条件 |
|---|---|
| TDD | 关键路径先红后绿 |
| Contract | 三条路由与 SSE event 符合父任务合同 |
| Security | 响应/日志/模板/diff 无真实密钥 |
| Compat | knowledge 路径与 1536 Embedding 不变 |
| Check | trellis-check 通过 |

## Risk Points

- Eino 实际 API 与调研摘要不一致 → 实现时以 Context7/源码签名为准。
- Result 外壳抽取时避免 knowledge 与 agent 互相导入。
- SSE 缓冲与取消语义容易漏测。
- 根 Router 迁移破坏旧测试。

## Rollback Points

1. 配置改动可独立回退。
2. agent 包可整删。
3. Router 迁移失败时恢复 knowledge 原 `NewRouter`。
4. 无数据库迁移，无需 schema 回滚。
