# Eino Agent 与 Vue 对话界面实施计划

## Preconditions

- 父任务与两个子任务仍为 `planning`。用户审批本文件及子任务 `prd.md`/`design.md`/`implement.md` 后，才能 `task.py start`。
- 实施顺序固定：
  1. `07-27-eino-agent-backend`
  2. `07-27-eino-chat-frontend`（优先 Kimi K3，不可用时 K2.7 Code）
  3. 父任务最终集成验收
- 工作分支：`feature/eino-agent-chat-ui`
- 父任务不直接写业务产品代码；只冻结跨端合同、顺序与集成验收。

## Implementation Checklist

### A. 规划收口（父任务）

1. 确认父任务 `prd.md`、`design.md`、本 `implement.md` 完整。
2. 确认子任务规划产物齐全：
   - backend: `prd.md` + `design.md` + `implement.md`
   - frontend: `prd.md` + `design.md` + `implement.md`
3. 配置三套 `implement.jsonl` / `check.jsonl` 真实条目。
4. 用户审查通过后，先启动后端子任务。

### B. 后端子任务（`07-27-eino-agent-backend`）

1. 先写失败测试，再实现：
   - 配置校验与密钥脱敏
   - domain 消息/模型不变量
   - application 编排与取消
   - Eino adapter（本地 `httptest.Server`）
   - HTTP JSON/SSE 合同
   - 组合根构造顺序与清理
2. 根 Router 迁移：
   - `platform/httpserver.NewRouter`
   - knowledge/agent `RegisterRoutes`
   - 保留原有 `/health` 与全部 knowledge 路径
3. 新增 `internal/agent/{domain,application,adapter/eino,adapter/http}`。
4. 更新 `backend/.env.example`、`docker/env.example`、`docker/docker-compose.yml`、后端 README。
5. 验证：
   ```powershell
   cd backend
   go fmt ./...
   go test ./...
   go vet ./...
   go build -o ../.trellis/workspace/bin/travel-agent.exe ./cmd/travel-agent
   cd ..
   git diff --check
   ```
6. 后端通过 `trellis-check` 后，再切换前端子任务。

### C. 前端子任务（`07-27-eino-chat-frontend`）

1. 优先调用 Kimi K3；不可用时改用 K2.7 Code。两者都不可用才报告并暂停。
2. Kimi K3 或 K2.7 Code 实现：
   - 依赖安装与 Element Plus namespace 主题
   - Router `/chat` `/models`
   - Ant Design Vue 应用壳与对话页
   - Element Plus 模型绑定页
   - Vuetify 局部 `ModelStatusPanel`
   - Pinia store、API 客户端、SSE 解析、JSON 降级
   - 单元测试、Playwright mock E2E、前端 README
3. 主流程复审合同、边界和测试，不抢先写产品页面。
4. 验证：
   ```powershell
   cd frontend
   pnpm lint
   pnpm test:unit -- --run
   pnpm build
   pnpm test:e2e --project=chromium
   ```

### D. 父任务最终集成

1. 跨端联调：模型目录、SSE 流式、JSON 降级、停止生成、失效模型回退。
2. 安全检查：API Key 不出现在响应、日志、浏览器存储、Git diff。
3. Compose 配置解析与文档一致性。
4. 知识接口与 Embedding 维度回归不破坏。
5. 父任务 `trellis-check`、spec 更新、提交。

## Validation Commands

后端：

```powershell
cd backend
go fmt ./...
go test ./...
go vet ./...
go build -o ../.trellis/workspace/bin/travel-agent.exe ./cmd/travel-agent
```

前端：

```powershell
cd frontend
pnpm lint
pnpm test:unit -- --run
pnpm build
pnpm test:e2e --project=chromium
```

仓库根：

```powershell
git diff --check
git status --short
```

## Review Gates

| Gate | 条件 | 失败动作 |
|---|---|---|
| Plan review | 三套 prd/design/implement + jsonl 就绪 | 停留 planning |
| Backend start | 用户确认后端先做 | 不 start 前端 |
| Backend done | 后端测试/构建/检查通过 | 不进入前端 |
| Frontend start | Kimi K3 或 K2.7 Code 可用 | 两者都不可用则报告并暂停前端 |
| Frontend done | 前端测试/构建/E2E 通过 + 主流程复审 | 不进入父集成 |
| Parent done | 跨端/安全/Docker/文档/兼容验收通过 | 回滚或修复后重验 |

## Risk Points

- 根 Router 迁移可能破坏 knowledge 路由；必须有兼容测试。
- Eino API 需按实际依赖版本核验签名，不以调研摘要硬编码。
- SSE 代理缓冲与 Context 取消需专门测试。
- 三套 UI 库样式冲突，必须严格分区 + Element Plus namespace CSS。
- 前端模型优先 Kimi K3，仅允许明确回退到 K2.7 Code，不能静默替代为其他模型。

## Rollback Points

1. 后端 Agent 上下文可整体删除，不涉及迁移。
2. 根 Router 出问题优先恢复 knowledge 原路径注册。
3. 前端可回退脚手架，不影响后端 API。
4. 前端未开始时，后端可单独保留等待。
