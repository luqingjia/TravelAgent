# 初始化 Eino Agent 与 Vue 对话模型界面

## Goal

在现有 TravelAgent 单仓库中新增一个最小但可扩展的 AI Agent MVP：后端使用 CloudWeGo Eino 和 OpenAI 兼容 ChatModel，提供只读模型目录、普通 JSON 对话和 SSE 流式对话；前端使用 Vue 3 构建对话页与模型绑定页，并按明确边界组合 Ant Design Vue、Element Plus 和 Vuetify。功能不得破坏现有知识文档摄取、Embedding、PostgreSQL/pgvector 和 Docker 部署能力。

所有规划与代码都位于分支 `feature/eino-agent-chat-ui`。后端由主实现流程完成；`frontend/` 产品代码优先由 Kimi K3 编写；若 Kimi K3 不可用，允许改用 K2.7 Code，不得再静默换成其他模型。

## Background

- Go 后端位于 `backend/`，当前只有知识文档上传、处理、查询、删除和 Embedding 能力，没有 Eino、ChatModel、Agent、Tool Calling 或会话 API。
- 后端采用模块化单体、轻量 DDD 和手工构造器注入，`internal/app` 是唯一组合根。
- Vue 前端位于 `frontend/`，已有 Vue Router、Pinia、Vitest、Playwright、ESLint 和 Vite，但仍是脚手架页面，没有 API 客户端、业务布局或对话状态。
- 当前 Embedding 配置不能复用为 ChatModel 配置；聊天模型需要独立的模型目录、密钥和超时合同。
- 根目录 `docker/` 独立编排后端和 PostgreSQL；新增聊天配置必须同步到环境变量模板和 Compose。

## Child Tasks

1. `07-27-eino-agent-backend`：模型配置、Eino Agent、时间工具、JSON/SSE API、后端测试与文档。
2. `07-27-eino-chat-frontend`：由 Kimi K3（不可用时改用 K2.7 Code）实现 Vue 应用壳、对话页、模型绑定页、状态管理、API 客户端和前端测试。
3. 父任务负责冻结跨端合同、确定实施顺序并执行最终集成验证，不重复承载子任务产品代码。

## Requirements

1. 后端引入 CloudWeGo Eino 官方模块，使用 Eino `ChatModelAgent` 和 `Runner` 完成真实 Agent 循环，不以裸 HTTP Chat Completions 冒充 Agent。
2. ChatModel 使用 OpenAI 兼容协议。可用模型由后端环境变量预配置，至少包含稳定模型 ID、显示名称、供应商、Base URL、模型名、API Key、超时和是否启用。
3. API Key 不返回前端、不写日志、不进入数据库、不经过浏览器，也不能出现在错误响应和模型元数据中。
4. 新增独立 `agent` 业务边界，不能把 Agent 逻辑塞进 `knowledge`，领域/应用核心不得依赖 Gin 或 Eino 具体类型。
5. 后端提供只读模型目录 `GET /api/agent/models`，返回可公开的模型元数据、默认模型 ID、配置可用状态和能力标记。状态明确表示“配置可用”，不伪装成实时外部网络探测。
6. 后端提供 `POST /api/agent/chat`，请求携带模型 ID和受限消息历史，响应使用现有 `code/message/data` 外壳一次返回完整助手消息。
7. 后端提供 `POST /api/agent/chat/stream`，请求合同与普通接口一致，使用 SSE `message`、`done`、`error` 三类事件输出增量内容、完成信息或可公开错误。
8. 两种对话路径共享同一输入校验、模型选择、应用服务、Eino Runner 和错误分类；不得维护两套互相漂移的业务逻辑。
9. SSE 开始前的参数或模型错误返回普通 JSON 4xx；流开始后的运行错误通过 `error` 事件发送。响应禁用代理缓冲，客户端断开必须通过 `context.Context` 取消模型调用。
10. Agent 注册内部工具 `get_current_time`。工具接收 IANA 时区名，使用标准库 `time.LoadLocation` 返回当地时间；无效时区产生可分类错误。工具不访问外部网络。
11. 后端保持无状态，不新增会话表或消息表。前端只在当前页面生命周期维护消息，每次请求携带受限最近历史；刷新后清空消息。
12. 后端必须限制消息数量、单条消息字符数和总字符数；只接受 `user`、`assistant` 历史角色，最后一条必须是非空 `user` 消息。
13. 前端只把所选模型 ID 保存到 `localStorage`。若保存的模型已禁用或消失，自动回退后端默认模型并显示提示；浏览器存储中不得出现密钥或 Base URL 凭据。
14. 对话页包含消息区、输入区、发送/生成中状态、停止按钮、错误提示、当前模型展示和模型绑定入口。流式增量更新同一条助手消息，不能每个 token 新建气泡。
15. 前端默认调用 SSE。只有在尚未收到有效增量内容时发生传输或协议失败，才自动降级 JSON；收到后端 `error` 事件或已有部分回复后不得自动重发，避免重复扣费和重复回答。
16. 前端使用 Vue Router 提供 `/chat` 和 `/models`，根路径重定向 `/chat`；Pinia 集中管理模型、消息、AbortController 和请求状态；集中 API 客户端统一读取 API Base URL。
17. 三套组件库按优势分工：Ant Design Vue 负责全局应用壳、导航和对话页；Element Plus 负责模型列表、筛选和选择交互；Vuetify 只用于模型详情中的配置状态/能力面板。
18. Element Plus 使用自定义 namespace 并配套生成对应主题 CSS；Vuetify 只在局部状态组件中渲染。禁止在同一表单、弹窗或消息列表中无规则混搭三套组件。
19. `frontend/` 产品代码优先由 Kimi K3 实现；若 Kimi K3 不可用，明确改用 K2.7 Code 并在任务记录中注明。不得静默换成除此之外的其他模型。
20. 新增后端与前端测试，覆盖配置校验、模型脱敏、模型选择、Agent 编排、时区工具、JSON/SSE 合同、流式解析、降级规则、停止生成和失效模型回退。
21. 更新 `backend/.env.example`、`docker/env.example`、`docker/docker-compose.yml`、根 README、后端 README 和前端 README，说明模型配置、接口、启动顺序与安全边界。
22. 不执行数据库迁移，不修改现有 `rag` schema 和 `vector(1536)`，现有知识文档接口保持兼容。

## API Contract

### Model Catalog

```http
GET /api/agent/models
```

```json
{
  "code": "0",
  "message": "",
  "data": {
    "defaultModelId": "qwen-plus",
    "models": [
      {
        "id": "qwen-plus",
        "displayName": "Qwen Plus",
        "provider": "dashscope",
        "model": "qwen-plus",
        "available": true,
        "capabilities": ["chat", "streaming", "tools"]
      }
    ]
  }
}
```

### Chat Request

```json
{
  "modelId": "qwen-plus",
  "messages": [
    {"role": "user", "content": "现在东京几点？"}
  ]
}
```

`modelId` 为空时使用后端默认模型。响应中的模型信息只能包含公开字段。

### JSON Chat

```http
POST /api/agent/chat
Content-Type: application/json
```

成功数据至少包含 `modelId` 和一条 `role=assistant` 的完整消息。

### SSE Chat

```http
POST /api/agent/chat/stream
Content-Type: application/json
Accept: text/event-stream
```

事件类型固定为：

```text
event: message
data: {"content":"增量文本"}

event: done
data: {"modelId":"qwen-plus"}

event: error
data: {"code":"B100001","message":"agent request failed"}
```

## Acceptance Criteria

- [x] `backend/go.mod` 包含实际使用的 Eino 与 OpenAI 模型适配依赖，源码存在可测试的 `ChatModelAgent`、Runner 和工具调用路径。
- [x] 模型目录从后端配置生成，默认模型合法，API 响应、日志、浏览器存储和 Git diff 均不包含真实 API Key。
- [x] JSON 与 SSE 接口合同可用，输入限制一致，客户端取消能够停止后端请求上下文。
- [x] `get_current_time` 能处理有效 IANA 时区并分类无效时区错误；Agent 能在测试中完成工具调用后生成最终回答。
- [x] 后端不新增会话/消息表，多轮对话只由前端携带受限历史完成。
- [x] 前端存在可导航的 `/chat` 和 `/models`，刷新清空消息但恢复仍有效的模型选择。
- [x] Ant Design Vue、Element Plus、Vuetify 均为直接依赖且按约定分区；Element Plus 自定义 namespace 的组件与 CSS 一致。
- [x] SSE 增量只更新一个助手气泡；停止按钮取消请求；降级规则不会在已有部分输出后重复请求。
- [x] 前端产品代码由 Kimi K3 或经批准的 K2.7 Code 实现，并经过主流程集成复审。
- [x] 后端 `go fmt ./...`、`go test ./...`、`go vet ./...`、build 通过。
- [x] 前端 `pnpm lint`、`pnpm test:unit -- --run`、`pnpm build` 和关键 Playwright E2E 通过。
- [x] Compose 默认与 S3 profile 配置解析通过，文档与模板只含安全占位值。
- [x] 现有知识接口、Embedding 维度和 Docker 基线行为保持兼容，`git diff --check` 通过。

## Out of Scope

- 生产级登录鉴权、多租户、计费、配额和审计后台。
- 多 Agent 协作、复杂工作流、长期记忆、会话搜索、分享、附件、多模态和消息编辑。
- 把模型密钥写入数据库或提供浏览器录入密钥的管理页面。
- RAG 检索增强或把现有知识库自动接入 Agent；后续任务再设计。
- 供应商实时健康探测或定时探活；首版状态只表示配置是否可使用。
