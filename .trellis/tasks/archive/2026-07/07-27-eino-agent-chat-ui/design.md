# Eino Agent 与 Vue 对话界面总体技术设计

## 1. 设计目标

在现有 `backend/` 与 `frontend/` 单仓库中增加一个最小 Agent 对话闭环，同时保持知识文档摄取能力不变：

- 后端使用 CloudWeGo Eino `ChatModelAgent`、Runner 和 OpenAI 兼容 ChatModel。
- 浏览器从后端读取脱敏模型目录，默认通过 SSE 获得增量回答，也可使用 JSON 完整回答。
- 后端不保存会话；前端只保留当前页面历史，并把受限历史随请求发送。
- `get_current_time` 工具证明首版确实具备 Agent 工具调用能力。
- 前端按边界使用 Ant Design Vue、Element Plus、Vuetify，避免样式和交互体系无规则混搭。
- 所有产品代码位于 `feature/eino-agent-chat-ui`；前端产品代码优先由 Kimi K3 编写，不可用时改用 K2.7 Code。

## 2. 任务边界与实施顺序

父任务只拥有跨端合同、顺序和最终集成，不重复实现子任务产品代码：

1. 后端子任务先冻结并实现模型目录、JSON/SSE 对话 API。
2. 后端合同与测试稳定后，切换前端子任务。
3. 前端子任务优先用 Kimi K3 实现页面、状态和 API 客户端；Kimi K3 不可用时改用 K2.7 Code。
4. 子任务分别完成检查后回到父任务执行跨端、Docker、文档和安全验收。

父子任务不是并行写同一文件的机制。前端依赖后端合同，必须后端接口稳定后再实施。

## 3. 总体架构

```text
frontend/
  Router + Pinia + API client
      | GET models / POST JSON / POST SSE
      v
backend/internal/platform/httpserver   通用 Router / middleware / Result
      |
      +--> knowledge HTTP adapter --> knowledge application --> existing ports
      |
      +--> agent HTTP adapter -----> agent application -----> AgentRuntime port
                                                        |
                                                        v
                                             Eino adapter / model catalog
                                                        |
                                                        v
                                            OpenAI-compatible providers
```

### 3.1 后端上下文

新增 `internal/agent` 限界上下文：

- `domain`：公开模型元数据、对话消息、错误分类和不变量，只依赖标准库。
- `application`：模型选择、历史校验、运行编排和流式事件；定义 `Catalog`、`Runtime` 等小接口。
- `adapter/eino`：Eino、OpenAI ChatModel、Runner、工具注册和供应商协议。
- `adapter/http`：JSON DTO、SSE framing、错误映射与路由注册。

`knowledge` 与 `agent` 不互相导入。共享 HTTP 外壳和根 Router 归属 `platform/httpserver`，避免继续让 knowledge adapter 拥有全局路由树。

### 3.2 前端边界

```text
App.vue / AppShell (Ant Design Vue)
├── /chat   ChatView + message/input controls (Ant Design Vue)
└── /models ModelBindingView
    ├── model table/filter/selection (Element Plus, namespace tael)
    └── ModelStatusPanel subtree (Vuetify only)

Pinia agent store
  -> typed agent API client
  -> POST-SSE decoder / JSON fallback
```

组件只能依赖共享 TypeScript 类型、Store 和 API 客户端，不能各自解析后端原始 payload。

## 4. 跨端数据合同

### 4.1 统一响应外壳

非流式接口沿用：

```json
{"code":"0","message":"","data":{}}
```

失败始终包含 `code/message/data`，`data` 为 `null`。未知运行错误不公开供应商原始响应、Base URL、密钥或 prompt。

### 4.2 模型目录

`GET /api/agent/models` 返回：

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

`available` 只表示配置完整并已启用，不代表实时外部健康探测。

### 4.3 对话请求

JSON 与 SSE 共用：

```json
{
  "modelId": "qwen-plus",
  "messages": [
    {"role":"user","content":"现在东京几点？"}
  ]
}
```

约束：

- `modelId` 为空时使用后端默认模型。
- 角色只允许 `user`、`assistant`。
- 最后一条必须为非空 `user`。
- 后端执行消息数量、单条字符数、总字符数最终校验；前端裁剪只是体验优化。
- 前端不得发送 system message；系统指令由后端控制。

### 4.4 JSON 回答

`POST /api/agent/chat` 的 `data`：

```json
{
  "modelId": "qwen-plus",
  "message": {"role":"assistant","content":"..."}
}
```

### 4.5 SSE 回答

`POST /api/agent/chat/stream`：

```text
event: message
data: {"content":"增量文本"}

event: done
data: {"modelId":"qwen-plus"}

event: error
data: {"code":"B100001","message":"agent request failed"}
```

合同：

- 流开始前的绑定、JSON、模型选择或历史校验错误返回普通 JSON 4xx。
- 写出首个 SSE header/event 后不再修改 HTTP 状态；运行错误用 `error` 事件。
- 每个事件以空行结束并立即 flush。
- 客户端断开传播请求 Context 取消。
- 前端只在未收到任何有效 `message` 内容前的网络/协议故障进行 JSON 降级；`error` 事件、用户取消或已有部分文本均不重试。

## 5. 配置合同

新增后端环境变量：

```text
AGENT_MODELS_JSON
AGENT_DEFAULT_MODEL_ID
AGENT_MAX_HISTORY_MESSAGES
AGENT_MAX_MESSAGE_CHARS
AGENT_MAX_TOTAL_CHARS
AGENT_MAX_ITERATIONS
```

`AGENT_MODELS_JSON` 示例只使用占位密钥：

```json
[
  {
    "id":"qwen-plus",
    "displayName":"Qwen Plus",
    "provider":"dashscope",
    "baseURL":"https://dashscope.aliyuncs.com/compatible-mode/v1",
    "model":"qwen-plus",
    "apiKey":"replace-me",
    "timeout":"60s",
    "enabled":true
  }
]
```

配置在数据库连接和 ChatModel 创建之前解析、归一化并集中校验。日志与错误不能格式化整个模型配置结构。

前端配置：

```text
VITE_API_BASE_URL
```

空值表示使用同源路径；Vite 开发服务器将 `/api` 代理到 `http://localhost:8081`。

## 6. Eino 运行设计

每个启用模型创建独立 OpenAI ChatModel、`ChatModelAgent` 和 Runner，按稳定 ID 存入不可变运行目录。构造阶段完成失败，服务不得带半配置模型启动。

Agent 配置包含：

- 后端固定系统指令；
- `get_current_time` 工具；
- 有限 `MaxIterations`；
- OpenAI ChatModel 的 BaseURL、Model、APIKey、Timeout。

`get_current_time` 输入 `timezone`，通过 `time.LoadLocation` 校验 IANA 名称；输出时区、RFC3339 与可读当地时间。工具不联网，错误通过稳定 sentinel/typed error 分类。

应用层只消费框架无关的流事件：

```go
type StreamEvent struct {
    Kind    EventKind
    Content string
    ModelID string
}
```

Eino adapter 负责把 Runner event 转换成 `message/done/error` 意义，应用和 HTTP 层不依赖 Eino event 类型。

## 7. 根 Router 迁移

当前 `knowledge/adapter/http.NewRouter` 同时创建 Gin Engine、健康路由和知识路由，新增 Agent 后会造成上下文倒置。设计调整为：

- `platform/httpserver.NewRouter(middleware)`：创建 Engine、安装中间件、注册 `/health`。
- `knowledge/adapter/http.RegisterRoutes(router, handler)`：只注册 `/api/knowledge`。
- `agent/adapter/http.RegisterRoutes(router, handler)`：只注册 `/api/agent`。
- `internal/app` 创建根 Router，并依次注册两个上下文。

迁移必须保留原七条健康/知识路径和中间件顺序。

## 8. 前端状态与交互

Pinia store 保存：

- 模型目录和默认模型 ID；
- 当前模型 ID；
- 当前页面消息；
- `idle/loading/streaming` 状态；
- 当前错误、一次性回退提示；
- 是否收到有效流内容；
- 当前 AbortController。

只有模型 ID 写入 `localStorage`。Store 初始化时读取模型目录后验证保存值：有效则恢复，无效则选默认并提示。消息永不持久化。

发送顺序：

1. 验证输入、模型和非生成状态。
2. 追加 user 消息和一个空 assistant 占位。
3. 创建 AbortController，调用 SSE。
4. `message` 内容追加到同一 assistant 占位。
5. `done` 完成；`error` 保留已有内容并显示错误。
6. 符合安全条件时才删除空占位并调用 JSON 降级。
7. Stop 调用 abort，恢复可发送状态且不显示系统故障。

## 9. UI 库隔离

- Ant Design Vue：全局布局、导航、ChatView 的消息/输入/按钮/Alert/Spin。
- Element Plus：ModelsView 的筛选、表格/列表、单选绑定、配置提示；根部通过 `ElConfigProvider namespace="tael"`，Sass theme-chalk 配置使用相同 namespace。
- Vuetify：只在 `ModelStatusPanel.vue` 局部子树使用 card/chip/alert/skeleton，单独插件/主题入口，不接管全局 App 壳。

不在同一个表单、消息列表或弹窗混用三套组件。

## 10. 测试策略

### 后端

- domain：模型/消息不变量和错误分类。
- application：默认/指定模型、边界限制、共享 JSON/stream 路径、取消和运行错误。
- Eino adapter：本地 `httptest.Server` 模拟 OpenAI 兼容协议；时间工具有效/无效时区；不调用真实供应商。
- HTTP：模型目录脱敏、统一外壳、三条路由、SSE headers/framing/flush、流前 JSON 错误、流后 error。
- app/config：模型 JSON、秘密安全、构造顺序、清理、旧路由兼容。

### 前端

- API/SSE parser：跨 chunk、多事件、多行 data、不完整 frame、错误事件。
- Store：目录加载、localStorage、无效模型回退、单气泡增量、降级条件、Abort。
- 组件：发送禁用、停止、错误、三库边界与 namespace。
- Playwright：mock 模型目录和 SSE，验证模型选择与流式主路径。

## 11. 兼容、部署与安全

- 不新增数据库迁移或会话表，不修改 `rag` schema、知识路由和 1536 维 Embedding。
- Compose 为 `app` 注入 Agent 配置；模板只使用安全占位值。
- 本 MVP **不引入 nginx**。开发期用 Vite 代理透传 SSE；生产默认同源。服务端仍写 `X-Accel-Buffering: no` 与逐事件 flush，便于未来网关；无 nginx 时无副作用。
- 日志允许：request ID、模型 ID、工具名、耗时、错误链。
- 日志禁止：API Key、Authorization、Base URL 中凭据、完整 prompt、完整回答、供应商完整响应。

## 12. 回滚

- Agent 是新增上下文，无数据库变更；回滚代码和配置即可。
- 根 Router 重构若导致旧接口回归，优先恢复原路径注册并修复注册抽象，不修改旧 API。
- 前端可单独回滚到脚手架；后端 API 不依赖前端部署。
- 前端仅在 Kimi K3 与 K2.7 Code 都不可用时暂停；否则后端完成后继续前端。
