# TravelAgent Frontend

Vue 3 + TypeScript + Vite 前端：Agent 对话页与模型绑定页。

## 技术栈与 UI 边界

| 区域 | 库 |
|---|---|
| 应用壳 / 导航 / 对话消息与输入 | **Ant Design Vue** |
| 模型列表、筛选、单选绑定 | **Element Plus**（namespace `tael`，Sass 编译同名 CSS） |
| `ModelStatusPanel` 配置状态/能力 | **Vuetify**（仅局部子树） |

禁止在同一表单、弹窗或消息列表内混搭三套组件。

## 环境变量

见仓库模板 `env.example`（可复制为本地 `.env.local`，Vite 会自动加载 `.env*`）：

- `VITE_API_BASE_URL`：API 根地址。**留空表示同源**。
- **不要**写入任何模型 API Key。前端只接收后端脱敏模型元数据。

## 本地启动

1. 后端在 `http://localhost:8081` 提供 Agent API（或你自定义后同步修改代理）。
2. 前端：

```sh
pnpm install
pnpm dev
```

开发服务器将同源路径 `/api` **代理**到 `http://localhost:8081`，避免本地必须放宽后端 CORS。

### SSE / 无 nginx 说明

本仓库**不引入 nginx**。流式对话约定：

1. **后端**：`text/event-stream`、`Cache-Control: no-cache`、及时 flush；可选 `X-Accel-Buffering: no`（预留网关，无 nginx 时无副作用）。
2. **前端开发**：Vite `server.proxy['/api']` 转发到 `http://localhost:8081`，并透传流式响应（不对 `/api/agent/chat/stream` 做缓冲聚合）。
3. **前端消费**：`fetch` + `ReadableStream` 解析 SSE；**禁用** `EventSource`（仅支持 GET）。
4. **生产**：默认 `VITE_API_BASE_URL` 为空，与后端同源部署；分域时再设置后端 origin。仍不在仓库内添加 nginx 配置。

## 路由

- `/` → 重定向 `/chat`
- `/chat` 对话页
- `/models` 模型绑定页
- 未知路径 → `/chat`

## 状态与持久化

- Pinia `agent` store 管理模型目录、选中模型、当前页消息、发送状态、错误、流内容标记与 `AbortController`。
- **仅** `selectedModelId` 写入 `localStorage`。
- **消息不持久化**：刷新后清空。

## 对话行为摘要

- 发送时追加用户消息 + 空助手占位；SSE `message` 增量写入同一助手气泡。
- `done` 结束；`error` 保留已显示文本并展示错误，**不**自动重发。
- 仅当**尚未收到任何有效流内容**且发生网络/协议失败时，才删除空占位并降级 `POST /api/agent/chat` JSON。
- 停止按钮调用 `AbortController`；取消不显示为系统故障。

## 脚本

```sh
pnpm lint
pnpm test:unit -- --run
pnpm build
pnpm test:e2e --project=chromium
```

首次 e2e 需安装浏览器：`npx playwright install chromium`。

## 后端合同（只读依赖）

- `GET /api/agent/models`
- `POST /api/agent/chat`
- `POST /api/agent/chat/stream`（SSE：`message` / `done` / `error`）
- 响应外壳：`{ code, message, data }`
- 公开模型字段：`id`, `displayName`, `provider`, `model`, `available`, `capabilities[]`
