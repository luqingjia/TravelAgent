# 实现 Eino Agent 与对话 API

## Goal

在 `backend/` 新增独立 Agent 业务边界，使用 CloudWeGo Eino `ChatModelAgent`、OpenAI 兼容 ChatModel 和 `get_current_time` 工具，实现安全的模型目录、普通 JSON 对话和 SSE 流式对话，并保持现有知识文档业务兼容。

## Requirements

1. 新增 `internal/agent/domain`、`internal/agent/application`、`internal/agent/adapter/eino` 和 `internal/agent/adapter/http`，依赖方向由适配器指向应用/领域。
2. `domain` 只依赖标准库；`application` 定义自己需要的模型运行时/目录小接口，不导入 Gin 或 Eino。
3. `adapter/eino` 使用官方 Eino 与 `eino-ext/components/model/openai` 创建每个启用模型的 ChatModel、`ChatModelAgent`、Runner 和工具集合。
4. 模型配置来自 `AGENT_MODELS_JSON`，默认模型来自 `AGENT_DEFAULT_MODEL_ID`。每个模型包含 `id/displayName/provider/baseURL/model/apiKey/timeout/enabled`；ID 必须唯一且默认模型必须启用。
5. 配置读取还支持 `AGENT_MAX_HISTORY_MESSAGES`、`AGENT_MAX_MESSAGE_CHARS`、`AGENT_MAX_TOTAL_CHARS`、`AGENT_MAX_ITERATIONS`，提供安全默认值并在建立外部客户端前校验。
6. API 只公开模型 ID、显示名、供应商、模型名、配置可用状态和 `chat/streaming/tools` 能力，不公开 API Key 或带凭据 URL。
7. 应用服务验证模型选择和消息历史：只允许 `user/assistant`，最后一条必须是非空 `user`，并限制数量、单条字符数和总字符数。
8. 注册 `get_current_time` 工具，输入 `timezone` 为 IANA 时区；输出包含时区、本地 RFC3339 时间和可读时间；无效时区包装稳定错误。
9. 普通接口 `POST /api/agent/chat` 收集同一运行路径的增量并返回完整助手消息。
10. 流式接口 `POST /api/agent/chat/stream` 使用 SSE `message/done/error`。预检错误返回 JSON 4xx；开始流后不再改 HTTP 状态。
11. SSE 设置 `Content-Type: text/event-stream`、`Cache-Control: no-cache`、`Connection: keep-alive`、`X-Accel-Buffering: no`，每个事件及时 flush。
12. 浏览器断开或调用方取消时停止遍历 Eino 事件并传递 Context 取消；日志不得记录 prompt、完整回复、密钥或 Authorization。
13. 在 `internal/app` 组合根显式创建 Agent 运行时、服务和 Handler，并在统一 Gin Router 注册知识路由与 Agent 路由；不得使用全局容器或 `gin.Context` 服务定位。
14. 保持统一 `code/message/data` 外壳。Agent 客户端错误使用稳定 4xx 业务码，模型/Agent 运行失败只公开通用消息并保留服务端错误链。
15. 更新配置模板、Docker Compose 和后端文档；不执行数据库迁移。
16. 测试不得调用真实模型服务，使用 fake 应用端口和 `httptest.Server` 模拟 OpenAI 兼容响应。

## Acceptance Criteria

- [x] Eino 依赖被实际调用，不只是写入 `go.mod`。
- [x] 配置测试覆盖空模型、重复 ID、非法 URL/timeout、缺密钥、默认模型不存在、历史限制非法值和密钥不泄漏。
- [x] 模型目录按配置返回脱敏元数据，禁用模型不可选择。
- [x] 应用测试覆盖默认模型、指定模型、消息边界、Runner 错误、Context 取消和 JSON/流式共享路径。
- [x] 时间工具测试覆盖 UTC、`Asia/Shanghai` 和非法时区。
- [x] Eino 适配器通过本地假 OpenAI 服务验证普通回复，并验证工具注册/执行路径。
- [x] HTTP 测试覆盖三条路由、响应外壳、SSE headers/event framing、预检 JSON 错误和流后 error event。
- [x] 组合根测试覆盖 Agent 构造顺序和后续失败时数据库清理。
- [x] 现有后端测试全部通过，知识接口路径和 1536 维 Embedding 合同未改变。
- [x] `go fmt ./...`、`go test ./...`、`go vet ./...`、`go build -o ../.trellis/workspace/bin/travel-agent.exe ./cmd/travel-agent` 和 `git diff --check` 通过。

## Out of Scope

- 会话数据库、长期记忆和 RAG。
- 浏览器编辑模型或密钥。
- 多 Agent、Graph/Chain 工作流和外部联网工具。
- 实时模型供应商健康探测。
