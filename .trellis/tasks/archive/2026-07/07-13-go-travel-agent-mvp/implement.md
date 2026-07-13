# Go 版 TravelAgent MVP 实现计划

## Preconditions

- 当前任务仍处于 planning 状态，实施前需要用户审批 `prd.md`、`design.md`、`implement.md`。
- 实施前加载 `trellis-before-dev`，读取后端规范。
- 不删除 Java 项目，不修改 Maven 模块结构。

## Implementation Checklist

1. 建立 Go 模块骨架
   - 新增 `go/go.mod`。
   - 新增 `go/cmd/travel-agent/main.go`。
   - 新增 `internal/config`、`internal/http`、`internal/knowledge`、`internal/database`、`internal/embedding`、`internal/storage`。
   - 默认端口使用 `8081`。

2. 建立基础 HTTP 服务
   - 使用 Gin 初始化 router。
   - 增加健康检查接口，例如 `GET /health`。
   - 增加统一响应结构，尽量兼容 Java `Result<T>`。
   - 增加统一错误处理，区分参数错误、业务错误、系统错误。

3. 建立配置加载
   - 从环境变量读取 PostgreSQL、RustFS/S3、Embedding、上传限制。
   - 提供默认值：端口 `8081`，上传大小 `50MB`，Embedding 维度 `1536`。
   - 不提交真实密钥。

4. 建立数据库访问层
   - 使用 `sqlx + pgx driver` 连接 PostgreSQL。
   - 固定使用现有 `rag` schema 表。
   - 实现知识库存在检查。
   - 实现文档 CRUD、分页、状态查询、内容哈希去重。
   - 实现 chunk 和 vector 的批量替换。
   - 实现文档 processing 原子抢占。

5. 建立对象存储适配
   - 实现上传文件到 RustFS/S3。
   - 实现按 `source_uri` 读取文件。
   - 实现数据库保存失败时的补偿删除。
   - 如本地环境不具备 RustFS，可提供本地文件存储实现作为开发 fallback，但 README 必须写清楚。

6. 实现上传接口
   - `POST /api/knowledge/bases/{kb-id}/documents/upload`
   - 校验文件大小和扩展名。
   - 计算 SHA-256。
   - 同知识库重复内容直接返回业务错误。
   - 上传对象存储后保存 `pending` 文档。

7. 实现显式分块接口
   - `POST /api/knowledge/documents/{doc-id}/chunk`
   - 支持从 `pending` 或 `failed` 进入 `processing`。
   - 慢操作放在数据库事务外。
   - 成功时事务内替换 chunks、vectors、更新 `completed`。
   - 失败时更新 `failed` 和 `metadata.lastError`。

8. 实现文本解析和分块 MVP
   - 第一版优先支持 `txt`、`md`、`markdown`、`html`、`htm`。
   - `pdf`、`doc`、`docx` 如果 Go 依赖成本过高，可保留上传能力但在分块时返回明确错误；如果实现成本可控，再补解析。
   - 分块策略优先实现结构感知简化版，保留 chunk index、char count、start/end position。

9. 实现 Embedding
   - 定义 Embedding 接口。
   - 生产实现调用真实 Embedding API。
   - 测试实现返回固定 1536 维 fake 向量。
   - 写入 pgvector 时显式校验维度为 1536。

10. 补齐文档查询和删除接口
    - `GET /api/knowledge/documents/{doc-id}`
    - `GET /api/knowledge/documents/{doc-id}/status`
    - `GET /api/knowledge/bases/{kb-id}/documents`
    - `DELETE /api/knowledge/documents/{doc-id}`

11. 重写 README
    - 使用真实仓库结构。
    - 分开说明 Java 版和 Go MVP。
    - 写清楚数据库、环境变量、启动、测试、打包命令。
    - 写清楚 MVP 支持范围和暂不支持范围。

12. 验证
    - Java 侧执行 Maven 测试和打包。
    - Go 侧执行 `go test ./...` 和 `go build ./cmd/travel-agent`。
    - 如有集成测试环境，验证 pgvector 1536 维写入。
    - 检查 `git diff --check`。

## Validation Commands

从仓库根目录执行：

```powershell
mvn -q test
mvn -q clean package
```

从 `go/` 目录执行：

```powershell
go test ./...
go build ./cmd/travel-agent
```

最终提交前执行：

```powershell
git diff --check
git status --short
```

## Risk Points

- pgvector 写入：Go 没有 MyBatis-Plus，vector 需要用 SQL 明确 cast，必须测试 1536 维写入。
- 文件解析：Java 依赖 Spring AI/Tika，Go 侧 Office/PDF 解析可能依赖额外库，MVP 需要控制范围。
- 对象存储：对象存储和数据库不是同一个事务，必须保留补偿删除。
- 接口兼容：Go 复用 Java 路径，响应结构需要尽量接近 Java `Result<T>`。
- README：不能继续保留不存在的 `infra`、`mcp` 描述。

## Rollback Points

- Go 模块骨架单独提交时，可直接删除 `go/` 回滚。
- README 单独或随 Go MVP 提交时，回滚不会影响 Java 代码运行。
- 如果数据库 SQL 需要变化，必须单独评审；本计划默认不改现有表。

## Review Gate

用户确认以下文件后，才能执行 `task.py start` 进入实现：

- `.trellis/tasks/07-13-go-travel-agent-mvp/prd.md`
- `.trellis/tasks/07-13-go-travel-agent-mvp/design.md`
- `.trellis/tasks/07-13-go-travel-agent-mvp/implement.md`
