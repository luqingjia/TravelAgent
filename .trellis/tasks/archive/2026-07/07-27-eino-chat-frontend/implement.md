# Vue 对话与模型绑定界面实施计划

## Preconditions

- 后端子任务已完成且合同稳定。
- 本任务 `prd.md` + `design.md` + 本文件审批后才可 start。
- **产品代码优先由 Kimi K3 编写；不可用时改用 K2.7 Code**。
- 若当前环境无法调用 Kimi K3：改用 K2.7 Code。两者都不可用时停止并向用户报告，不得再换其他模型。
- 主流程只做合同冻结、复审、测试修复，不抢先写页面产品代码。
- 工作分支：`feature/eino-agent-chat-ui`。

## Implementation Checklist

1. **分派 Kimi K3（或 K2.7 Code）**
   - 提示首行：`Active task: .trellis/tasks/07-27-eino-chat-frontend`
   - 明确要求实现本 PRD/设计/计划，不得改写后端。

2. **依赖与主题**
   - 安装 Ant Design Vue、Element Plus、Vuetify 及主题所需 Sass 依赖。
   - Element Plus namespace `tael` + 生成同名 CSS。
   - Vuetify 仅局部插件入口。
   - 提交 `pnpm-lock.yaml`。

3. **基础设施**
   - 类型定义对齐后端公开合同。
   - `api/agent.ts` + `api/sse.ts`。
   - Vite `/api` 代理到 `http://localhost:8081`（无 nginx；保证流式透传）。
   - `VITE_API_BASE_URL` 支持；空值同源。
   - **不引入 nginx**；SSE 依赖后端 flush/headers + 浏览器 fetch 流 + Vite 开发代理。

4. **Pinia store**
   - 模型目录、选择、消息、状态、错误、Abort、流内容标记。
   - localStorage 仅模型 ID。
   - 失效模型回退提示。

5. **路由与页面**
   - `/` → `/chat`
   - ChatView（Ant Design Vue）
   - ModelsView（Element Plus）
   - ModelStatusPanel（Vuetify only）

6. **对话行为**
   - 单气泡流式增量
   - Stop abort
   - 安全 JSON 降级
   - 空输入/生成中禁用发送

7. **测试**
   - parser / store / 组件单测
   - Playwright mock 主路径
   - 确认构建产物与源码无 API Key

8. **文档**
   - 更新 frontend README / env 说明

9. **主流程复审与质量门**
   - 审查三库边界、降级规则、存储安全
   - `pnpm lint`
   - `pnpm test:unit -- --run`
   - `pnpm build`
   - `pnpm test:e2e --project=chromium`
   - `trellis-check`

## Validation Commands

```powershell
cd frontend
pnpm lint
pnpm test:unit -- --run
pnpm build
pnpm test:e2e --project=chromium
```

## Review Gates

| Gate | 通过条件 |
|---|---|
| Model gate | 产品代码证明由 Kimi K3 或 K2.7 Code 生成 |
| Contract | 仅调用公开模型字段与约定 SSE event |
| UI boundary | 三库分区 + Element namespace CSS 匹配 |
| Safety | localStorage/源码/构建产物无密钥 |
| Tests | lint/unit/build/e2e 通过 |

## Risk Points

- Kimi K3 与 K2.7 Code 都不可用导致任务阻塞。
- 三套 UI CSS 互相污染。
- SSE 解析边界条件。
- 误以为需要 nginx `proxy_buffering off`；本任务无 nginx，应配置 Vite 代理与 fetch 流。
- 错误降级导致重复请求/重复计费。

## Rollback Points

1. 依赖安装失败可回退 package.json/lock。
2. 页面实现失败可回退到脚手架路由。
3. 不影响后端与数据库。
