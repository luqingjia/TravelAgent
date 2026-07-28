# 实现 Vue 对话与模型绑定界面

## Goal

由 Kimi K3（不可用时改用 K2.7 Code）在 `frontend/` 实现可用的 Agent 对话页和模型绑定页：Ant Design Vue 承担应用壳与聊天体验，Element Plus 承担模型列表和选择交互，Vuetify 承担局部模型配置状态/能力面板；通过 Pinia 和集中 API 客户端对接后端 JSON/SSE 合同。

## Execution Constraint

- 本子任务的产品代码优先由 Kimi K3 编写；Kimi K3 不可用时明确改用 K2.7 Code。
- 分派时必须明确指定 Kimi K3 或 K2.7 Code，并在提示首行包含当前 Trellis 活动任务路径。
- 若执行环境无法提供 Kimi K3，改用 K2.7 Code；两者都不可用时停止并向用户报告，不得再静默改用其他模型。
- 主流程负责 API 合同、代码复审、测试和集成修复，但不得在分派前抢先实现页面。

## Requirements

1. 安装并直接使用 Ant Design Vue、Element Plus、Vuetify；生成并提交 `pnpm-lock.yaml`。
2. 路由包含 `/chat` 和 `/models`，`/` 重定向 `/chat`，未知路径回到对话页。
3. Ant Design Vue 实现响应式应用壳、侧边/顶部导航、对话消息区、输入区、模型标识、加载状态、错误提示和停止按钮。
4. Element Plus 实现模型绑定页的列表/表格、筛选、单选绑定和提示。页面只能选择后端已有模型，不出现 API Key、新增或编辑供应商表单。
5. Element Plus 使用自定义 namespace（例如 `tael`），并通过 Sass 主题入口生成相同 namespace 的 CSS，不能只改 ConfigProvider 导致样式类不匹配。
6. Vuetify 只用于独立 `ModelStatusPanel.vue` 子树，展示配置可用状态、默认/当前模型和 `chat/streaming/tools` 能力。状态文案必须说明这不是实时网络探测。
7. Pinia `agent` store 管理模型目录、选中模型 ID、当前页消息、发送状态、错误、是否已收到流内容和 AbortController。
8. 只把选中模型 ID 保存到 `localStorage`；刷新后消息清空。保存的 ID 无效时使用后端默认模型并显示一次提示。
9. 集中 API 客户端统一处理 `GET /api/agent/models`、JSON chat 和 POST SSE；API Base URL 读取 `VITE_API_BASE_URL`，为空时使用同源路径。
10. POST SSE 使用 `fetch` + `ReadableStream` 解析跨 chunk 的 SSE frame，支持 `message/done/error` 和多行 data，不能使用只支持 GET 的 EventSource。
11. 发送时先追加用户消息和一个空助手占位；每个 `message` 增量追加到同一助手消息。`done` 结束；`error` 保留已显示文本并展示错误。
12. 只有在未收到任何有效流内容时发生网络/协议失败才降级 JSON；后端 `error` 事件或已有部分输出后不自动重发。
13. 停止按钮调用 AbortController，界面进入可再次发送状态；取消不显示成系统故障。
14. 客户端发送最近历史并做前端裁剪，但后端限制仍是最终权威。空输入、生成中重复发送和缺少可用模型时禁止发送。
15. Vite 开发环境支持将同源 `/api` 代理到 `http://localhost:8081`，避免本地开发必须放宽后端 CORS；生产部署可通过 `VITE_API_BASE_URL` 或反向代理配置。
16. 页面具备桌面和窄屏基本可用性，三套组件库不在同一表单、弹窗或消息列表内混搭。
17. 更新前端 README 和环境变量示例，说明启动、API 地址、组件库边界和消息不持久化。

## Acceptance Criteria

- [x] Git/任务执行记录能够证明前端产品代码由 Kimi K3 或经批准的 K2.7 Code 生成，并经过主流程复审。
- [x] `package.json` 和锁文件包含三套 UI 库及 Element Plus namespace 样式所需依赖。
- [x] `/chat`、`/models` 可直接访问和互相导航，刷新路由不出现空白应用。
- [x] 模型目录加载、默认选择、持久化选择和失效回退行为有 Pinia 单元测试。
- [x] SSE parser 单测覆盖拆包、多个事件、错误事件和不完整 frame。
- [x] 对话组件测试覆盖单气泡增量、停止、错误提示、发送禁用和降级条件。
- [x] Element Plus 的 DOM class 与编译 CSS 使用自定义 namespace；Vuetify 组件只出现在状态面板。
- [x] Playwright 使用网络 mock 验证模型选择和流式对话主路径，不依赖真实 API Key。
- [x] `pnpm lint`、`pnpm test:unit -- --run`、`pnpm build`、关键 `pnpm test:e2e --project=chromium` 通过。
- [x] 浏览器存储、前端源码和构建产物中不存在 API Key。

## Out of Scope

- 模型密钥录入、模型 CRUD 和供应商管理后台。
- 会话持久化、历史列表、搜索、分享、附件、多模态和 Markdown 高级编辑。
- 把三套组件库用于同一个控件区域以展示“全家桶”。
