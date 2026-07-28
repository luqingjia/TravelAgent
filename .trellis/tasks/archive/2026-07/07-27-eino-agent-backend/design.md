# Eino Agent 后端技术设计

## 1. 目标

在 `backend/` 新增独立 `agent` 限界上下文，提供：

- 只读模型目录
- JSON 完整对话
- SSE 流式对话
- `get_current_time` 工具调用

不破坏 knowledge 文档摄取、Embedding、PostgreSQL/pgvector 与 Docker 基线。

## 2. 包边界

```text
internal/agent/
  domain/          # 模型元数据、消息、错误分类；仅标准库
  application/     # 校验、选择模型、编排；定义 Catalog/Runtime 端口
  adapter/eino/    # Eino ChatModelAgent、Runner、OpenAI ChatModel、工具
  adapter/http/    # DTO、SSE framing、错误映射、RegisterRoutes
```

依赖方向：

- domain → stdlib only
- application → domain + 自己的接口
- adapter/* → application/domain + 外部库
- 只有 `internal/app` 组装具体实现

禁止：

- knowledge 与 agent 互相导入
- application/domain 导入 Gin 或 Eino 类型
- 全局 DI 容器 / gin.Context 服务定位

## 3. 根 Router 迁移

现状：`knowledge/adapter/http.NewRouter` 创建 Engine、中间件、`/health` 和 knowledge 路由。

目标：

```text
platform/httpserver.NewRouter(middleware)
  -> 创建 Engine、安装中间件、注册 /health

knowledge/adapter/http.RegisterRoutes(router, handler)
agent/adapter/http.RegisterRoutes(router, handler)
```

兼容要求：

- 中间件顺序：RequestID → AccessLog → Recovery
- `/health` 响应外壳不变
- 六条 knowledge 路径保持原样
- 组合根后续失败仍关闭数据库

## 4. 配置合同

环境变量：

```text
AGENT_MODELS_JSON
AGENT_DEFAULT_MODEL_ID
AGENT_MAX_HISTORY_MESSAGES   # default 20
AGENT_MAX_MESSAGE_CHARS      # default 4000
AGENT_MAX_TOTAL_CHARS        # default 16000
AGENT_MAX_ITERATIONS         # default 8
```

`AGENT_MODELS_JSON` 项：

```json
{
  "id": "qwen-plus",
  "displayName": "Qwen Plus",
  "provider": "dashscope",
  "baseURL": "https://dashscope.aliyuncs.com/compatible-mode/v1",
  "model": "qwen-plus",
  "apiKey": "replace-me",
  "timeout": "60s",
  "enabled": true
}
```

启动前校验：

- 至少一个模型
- ID 唯一
- 默认模型存在且 enabled
- baseURL 合法
- timeout 合法
- 启用模型必须有非空 apiKey
- 历史限制为正整数

密钥永不进入公开 DTO、日志、错误响应。

## 5. 应用层合同

### 5.1 端口

```go
type ModelCatalog interface {
    List() domain.CatalogView
}

type AgentRuntime interface {
    Chat(ctx context.Context, modelID string, messages []domain.Message) (domain.AssistantMessage, error)
    Stream(ctx context.Context, modelID string, messages []domain.Message) (<-chan StreamEvent, error)
}
```

`StreamEvent` 与框架无关：

```go
type StreamEvent struct {
    Kind    EventKind // message | done | error
    Content string
    ModelID string
    Code    string
    Message string
}
```

### 5.2 共享路径

`Chat` 与 `Stream` 共用：

1. 解析/默认模型
2. 校验消息角色、最后一条 user、数量/字符限制
3. 调用 Runtime
4. 错误分类

不得维护两套独立业务逻辑。

## 6. Eino 适配

每个 enabled 模型：

1. `openai.NewChatModel`（BaseURL/Model/APIKey/Timeout）
2. 注册 `get_current_time` 工具
3. `adk.NewChatModelAgent` + 有限 MaxIterations + 固定系统指令
4. `adk.NewRunner`
5. 以稳定 model ID 存入不可变运行目录

`get_current_time`：

- 输入：IANA timezone
- 校验：`time.LoadLocation`
- 输出：timezone + RFC3339 + 可读本地时间
- 不访问网络

测试：

- 使用 `httptest.Server` 模拟 OpenAI 兼容 chat/completions
- 不调用真实供应商

## 7. HTTP 合同

### 路由

- `GET /api/agent/models`
- `POST /api/agent/chat`
- `POST /api/agent/chat/stream`

### 统一外壳

```json
{"code":"0","message":"","data":{}}
```

### SSE

Headers：

```text
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no
```

Events：

```text
event: message
data: {"content":"..."}

event: done
data: {"modelId":"..."}

event: error
data: {"code":"...","message":"..."}
```

规则：

- 流开始前错误 → JSON 4xx
- 流开始后错误 → error event，不改状态码
- 每个事件后 flush
- 客户端断开 → Context cancel → 停止 Runner 消费

## 8. 错误分类

- 参数/历史非法：4xx 稳定业务码
- 模型不存在/不可用：4xx
- 工具非法时区：可分类业务错误，不泄漏内部堆栈
- Agent/模型运行失败：通用公开消息 + 服务端错误链

禁止公开：API Key、Authorization、Base URL 凭据、完整 prompt、完整供应商响应。

## 9. 组合根

`internal/app`：

1. 解析配置（含 Agent）
2. 打开数据库
3. 构造 knowledge 依赖
4. 构造 agent catalog/runtime/service/handler
5. 创建根 Router 并注册 knowledge + agent
6. 创建 HTTP server

任一后续步骤失败：关闭已打开数据库并聚合错误。

## 10. 测试矩阵

| 层 | 覆盖 |
|---|---|
| config | 空模型、重复 ID、非法 URL/timeout、缺密钥、默认模型非法、限制非法、密钥不泄漏 |
| domain | 角色、最后一条 user、限制、错误类型 |
| application | 默认/指定模型、边界、共享路径、取消、运行错误 |
| eino adapter | fake OpenAI 回复、工具有效/无效时区、迭代限制 |
| http | 三条路由、外壳、脱敏、SSE headers/framing、预检 JSON、流后 error |
| app | 构造顺序、失败清理、旧路由兼容 |

## 11. 文档与部署

更新：

- `backend/.env.example`
- `docker/env.example`
- `docker/docker-compose.yml`
- 后端 README / 根 README 相关段落

不执行迁移，不改 `rag` schema，不改 1536 维 Embedding。

## 12. 回滚

- 删除 `internal/agent` 与相关配置即可
- Router 回归时优先恢复 knowledge 路径
- 无数据库回滚问题
