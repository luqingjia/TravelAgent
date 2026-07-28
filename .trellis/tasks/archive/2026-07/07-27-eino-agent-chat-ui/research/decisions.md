# Eino 与三套 Vue UI 库调研摘要

## 官方 Eino 结论

- 核心模块：`github.com/cloudwego/eino`。
- OpenAI 兼容 ChatModel：`github.com/cloudwego/eino-ext/components/model/openai`。
- 官方构造形状：`openai.NewChatModel(ctx, &openai.ChatModelConfig{Model, APIKey, BaseURL, Timeout})`。
- Agent 使用 `adk.NewChatModelAgent`，运行使用 `adk.NewRunner`；Runner 以异步事件迭代器返回消息/动作/错误。
- `ChatModelAgentConfig.MaxIterations` 可限制模型与工具循环，首版必须设置有限值，不能沿用过大的隐式循环。
- 工具 schema 可通过官方 `utils.InferTool` 从有类型输入/输出函数推导；本任务工具是 `get_current_time`。
- Context 必须贯穿 ChatModel、Runner 和事件迭代；SSE 客户端断开后停止读取并取消模型调用。

官方参考：

- https://github.com/cloudwego/eino
- https://github.com/cloudwego/eino-ext
- https://www.cloudwego.io/docs/eino/

实现阶段必须根据最终拉取版本检查准确函数签名，不凭调研摘要猜测 API。

## 组件库分工结论

### Ant Design Vue

- 版本调研时 registry 最新稳定版：4.2.6。
- 用途：应用壳、导航、消息列表、输入与按钮、全局反馈。
- 不用于模型配置表格，避免和 Element Plus 重叠。

### Element Plus

- 版本调研时 registry 最新稳定版：2.14.3。
- 用途：模型列表、筛选、选择、Descriptions/Alert 等配置交互。
- 自定义 namespace 不能只配置 `ElConfigProvider`；必须通过 Sass theme-chalk 的 config mixin 生成同名 CSS。

### Vuetify

- 版本调研时 registry 最新稳定版：4.1.6。
- 用途：仅 `ModelStatusPanel` 局部子树中的 card/chip/alert/skeleton。
- 不承担全局布局或聊天控件，防止 Material reset 与其他库无边界竞争。

## 跨端合同决策

- 模型仅由后端环境变量配置；前端只收到非敏感元数据。
- 后端无状态；前端当前页携带受限历史，刷新清空消息。
- 前端只持久化模型 ID。
- API：`GET /api/agent/models`、`POST /api/agent/chat`、`POST /api/agent/chat/stream`。
- SSE event：`message`、`done`、`error`。
- 只有未收到流内容前的网络/协议错误才允许 JSON 降级；避免重复计费。
- 模型状态是“配置可用”，不是实时供应商健康探测。

## 执行约束

- 前端产品代码优先由 Kimi K3 编写；仅允许明确回退到 K2.7 Code，不允许静默换成其他模型。
- 后端和前端均先写失败测试，再做最小实现。
- 不调用真实模型服务做自动化测试；使用 fake 和本地 `httptest.Server`。
