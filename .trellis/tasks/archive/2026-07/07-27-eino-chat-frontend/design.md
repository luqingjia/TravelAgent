# Vue 对话与模型绑定界面技术设计

## 1. 目标

在 `frontend/` 实现可导航的 Agent 对话页与模型绑定页：

- Ant Design Vue：应用壳 + Chat
- Element Plus：模型列表/筛选/选择（namespace `tael`）
- Vuetify：仅 `ModelStatusPanel` 局部状态面板
- Pinia + 集中 API 客户端对接后端 JSON/SSE

执行约束：产品代码优先由 Kimi K3 编写；不可用时改用 K2.7 Code。

## 2. 依赖后端合同

前端依赖后端稳定后的：

- `GET /api/agent/models`
- `POST /api/agent/chat`
- `POST /api/agent/chat/stream`

公开模型字段：`id/displayName/provider/model/available/capabilities`。

不得请求或存储 API Key、Base URL 凭据。

## 3. 目录结构

```text
frontend/src/
  api/agent.ts              # 模型目录 / JSON chat / SSE chat
  api/sse.ts                # POST SSE frame 解析
  api/types.ts              # 与后端公开合同对齐的类型
  stores/agent.ts           # Pinia 状态
  layouts/AppShell.vue      # Ant Design Vue
  views/ChatView.vue        # Ant Design Vue
  views/ModelsView.vue      # Element Plus
  components/ModelStatusPanel.vue  # Vuetify only
  plugins/vuetify.ts
  styles/element/index.scss # namespace=tael 主题
  router/index.ts
```

## 4. 路由

```text
/        -> redirect /chat
/chat    -> ChatView
/models  -> ModelsView
/*       -> redirect /chat
```

## 5. 状态设计（Pinia）

状态：

- `models` / `defaultModelId` / `selectedModelId`
- `messages`（仅当前页）
- `status`: `idle | loading | streaming`
- `error`
- `fallbackNotice`（失效模型回退一次提示）
- `receivedStreamContent: boolean`
- `abortController`

持久化：

- 仅 `selectedModelId` → `localStorage`
- 消息永不持久化；刷新清空

初始化：

1. 拉模型目录
2. 读取 localStorage
3. 若 ID 仍可用则恢复，否则选默认并提示

## 6. API 客户端

- `VITE_API_BASE_URL` 为空时使用同源 `/api...`
- Vite dev 将 `/api` 代理到 `http://localhost:8081`
- 不使用 EventSource（只支持 GET）
- SSE 使用 `fetch` + `ReadableStream`，支持：
  - 跨 chunk 拆包
  - 多事件
  - 多行 data
  - 不完整 frame 缓存

## 6.1 无 nginx 的 SSE 约定

本任务**不引入 nginx 反向代理**。SSE 相关配置只依赖：

1. **后端**（已实现）：
   - `Content-Type: text/event-stream`
   - `Cache-Control: no-cache`
   - `Connection: keep-alive`
   - `X-Accel-Buffering: no`（预留给未来网关；无 nginx 时无副作用）
   - 每个 SSE 事件后立即 flush
   - 客户端断开通过 request `context` 取消 Runner
2. **前端开发**：
   - Vite `server.proxy['/api']` 转发到 `http://localhost:8081`
   - 代理必须透传流式响应，**不要**对 `/api/agent/chat/stream` 做响应缓冲聚合
   - 使用 `fetch` + `ReadableStream` 消费流；禁用 `EventSource`
3. **前端生产**：
   - 默认 `VITE_API_BASE_URL` 为空，与后端同源部署（例如同一 Compose 网络后的路径或静态资源同域）
   - 若前后端分域，才设置 `VITE_API_BASE_URL` 为后端 origin；仍不引入仓库内 nginx
4. **明确不做**：
   - 不在 `frontend/` 或仓库新增 nginx 配置
   - 不为 SSE 单独加 CORS 放宽以外的网关缓冲开关（无 nginx 时不需要 `proxy_buffering off`）

## 7. 发送与降级流程

1. 校验非空输入、可用模型、非生成中
2. 追加 user 消息 + 空 assistant 占位
3. 创建 AbortController，调用 SSE
4. `message`：追加到同一 assistant
5. `done`：结束
6. `error`：保留已显示文本 + 展示错误，不重发
7. 仅当未收到任何有效 `message` 内容且为网络/协议失败时，删除空占位并 JSON 降级
8. Stop：abort，恢复可发送；取消不当作系统故障

## 8. UI 库隔离

| 区域 | 库 |
|---|---|
| AppShell / 导航 / Chat 消息与输入 | Ant Design Vue |
| Models 列表、筛选、单选 | Element Plus (`namespace="tael"` + 同名 CSS) |
| ModelStatusPanel 状态/能力展示 | Vuetify |

禁止：

- 同一表单/弹窗/消息列表混搭三套组件
- 把 Vuetify 提升为全局 App 壳
- 只改 ConfigProvider 不生成 namespace CSS

## 9. 测试策略

- SSE parser：跨 chunk、多事件、错误事件、残缺 frame
- Store：目录、localStorage、失效回退、单气泡增量、降级条件、Abort
- 组件：发送禁用、停止、错误、namespace/边界
- Playwright：mock 模型目录与 SSE，覆盖选择模型与流式主路径

## 10. 文档

更新 frontend README：

- 启动顺序
- `VITE_API_BASE_URL`
- 组件库边界
- 消息不持久化
- 开发代理

## 11. 回滚

- 前端可独立回退脚手架
- 不依赖后端部署顺序以外的数据库变更
- Kimi K3 与 K2.7 Code 都不可用时本任务不开始
