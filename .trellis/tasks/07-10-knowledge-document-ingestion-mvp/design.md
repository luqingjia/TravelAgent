# 知识库文档摄取 MVP 技术设计

## 1. 设计结论

- 保留现有两个显式接口：上传接口只保存文件和文档记录，分块接口同步完成解析、切分、嵌入和持久化。
- 普通知识库、文档、分块和状态数据继续使用 MyBatis-Plus。
- 仅向量表参考 `ragent`：保留 `KnowledgeVectorService` 作为边界，底层改用 `JdbcTemplate` 和 PostgreSQL 原生 `jsonb`/`vector` 转换。
- 不新增执行记录表，不引入 MQ、异步线程池、Milvus、URL 抓取或前端改造。

该任务虽然包含上传、状态、数据库和对象存储多个交付点，但它们共同组成同一个事务与重试闭环，无法独立验收，因此维持一个任务按阶段实现，不拆分子任务。

## 2. 当前代码证据

- 上传入口已经存在：`KnowledgeDocumentController.java:30`。
- 显式分块入口已经存在：`KnowledgeDocumentController.java:37`。
- 当前上传在重复、扩展名和业务大小校验前就调用对象存储：`KnowledgeDocumentServiceImpl.java:74-112`。
- 当前处理在解析和嵌入完成后才进入数据库事务，具备保留旧数据的正确基础：`KnowledgeDocumentServiceImpl.java:175-234`。
- 当前失败信息会用只含 `lastError` 的新 Map 覆盖全部文档元数据，且成功后不会主动清除旧错误：`KnowledgeDocumentServiceImpl.java:236-247`。
- 当前向量 Mapper 继承 `BaseMapper<KnowledgeVectorEntity>`，而 `PGvector embedding` 没有 TypeHandler，导致应用上下文启动失败：`KnowledgeVectorMapper.java:14`、`KnowledgeVectorEntity.java:42-45`。
- 当前向量实际调用均为自定义 SQL，没有使用 `BaseMapper` 通用 CRUD；为该实体增加 TypeHandler 的收益有限。
- `ragent` 的 `VectorStoreService` 隔离存储边界，`PgVectorStoreService` 使用 `JdbcTemplate.batchUpdate` 和 `?::jsonb`、`?::vector` 批量写入：`C:/etc/ragent/bootstrap/src/main/java/com/nageoffer/ai/ragent/rag/core/vector/PgVectorStoreService.java:41-58`。

## 3. 总体数据流

```text
上传请求
  -> 校验知识库、文件名、扩展名、大小
  -> 读取内容并计算 SHA-256
  -> 校验同知识库重复内容
  -> 上传对象存储
  -> MyBatis-Plus 创建 pending 文档记录
  -> 若建表记录失败，补偿删除刚上传的对象

显式分块请求
  -> 条件更新文档状态为 processing
  -> 从对象存储读取文件
  -> 选择解析器并提取文本
  -> 使用现有策略生成分块
  -> 使用现有嵌入模型生成 1536 维向量
  -> 开启同一数据库事务
       -> 删除该文档上一轮分块和向量
       -> MyBatis-Plus 批量写入新分块
       -> JdbcTemplate 批量写入新向量
       -> 文档更新为 completed、更新数量、清除 lastError
  -> 任一步骤失败：事务回滚，旧数据恢复，文档更新为 failed + lastError
```

## 4. 上传设计

### 4.1 配置

在 `bootstrap` 的知识库业务包新增上传配置类，例如 `KnowledgeDocumentUploadProperties`：

- `allowedExtensions`：默认 `pdf,doc,docx,txt,md,markdown,html,htm`。
- `maxSize`：默认 `50MB`，使用 Spring `DataSize` 表达。
- 通过 `application.yml` 暴露环境变量覆盖入口，修改后重启生效。

配置属于知识库业务规则，放在 `bootstrap`，不放入通用 `framework`。

### 4.2 校验顺序

1. 确认知识库存在、文件非空且来源类型为文件。
2. 规范化文件扩展名为小写并检查允许列表。
3. 使用 `MultipartFile#getSize` 检查业务上限，避免先读取超大文件。
4. 读取文件内容并计算 SHA-256。
5. 按 `kb_id + content_hash + deleted=0` 查询重复。
6. 通过校验后才上传对象存储。
7. 创建文档记录；若数据库保存失败，尽力删除本次对象并重新抛出原异常。

服务层预查提供友好错误，数据库唯一索引解决两个并发请求同时通过预查的竞态。

## 5. 处理状态与重试

- 状态沿用 `pending / processing / completed / failed`。
- 进入处理时使用条件更新：仅当当前状态不是 `processing` 时才能切换成功；更新数量为 0 时返回“文档正在处理中”。
- 原有先查询再更新的检查可保留用于快速报错，但正确性由数据库条件更新保证。
- `processing` 时保留最近一次失败信息，方便观察正在重试的来源；只有本轮成功后清除 `lastError`。
- 失败时复制并合并原 metadata，再写入 `lastError`，不得清空其他键。
- 解析为空、分块为空、任一向量为空或维度不是 1536 时，在进入替换事务前失败。
- 进程被强制终止造成永久 `processing` 的自动恢复不在 MVP 内；正常 Java 异常必须进入 `failed`，之后允许显式重试。

## 6. 向量存储设计

### 6.1 边界

保留 `KnowledgeVectorService`，业务编排只依赖该接口。`KnowledgeVectorServiceImpl` 改为注入：

- `JdbcTemplate`
- `ObjectMapper`

普通表仍通过 MyBatis-Plus 访问。向量实现位于 `bootstrap`，因为它承载知识库向量的业务存储协议，而不是全项目通用数据库组件。

### 6.2 PostgreSQL 写入

批量 SQL 采用与 `ragent` 相同的机制，但保持 TravelAgent 的 schema 与 metadata 键：

```sql
INSERT INTO rag.t_knowledge_vector (id, content, metadata, embedding)
VALUES (?, ?, CAST(? AS jsonb), CAST(? AS vector))
ON CONFLICT (id) DO UPDATE SET
  content = EXCLUDED.content,
  metadata = EXCLUDED.metadata,
  embedding = EXCLUDED.embedding
```

- metadata 保持 `kbId / documentId / chunkId / chunkIndex`。
- `float[]` 在向量边界统一转换为 `[0.1,0.2,...]`，转换逻辑不散落到编排服务。
- `indexDocumentChunks` 使用 `JdbcTemplate.batchUpdate`，不再逐块调用单条 Mapper SQL。
- 删除文档向量和删除单个分块向量也收口到该实现内。
- `JdbcTemplate` 与 MyBatis-Plus 使用同一 `DataSource` 和 Spring 事务管理器，因此会参加 `TransactionOperations` 已开启的同一事务。

### 6.3 清理旧映射

- 移除不再使用的 `KnowledgeVectorMapper`，消除 MyBatis 对 `PGvector` 实体的启动期映射。
- 清理只为该 Mapper 服务的 `KnowledgeVectorEntity` 及未被调用的实体转换代码；若确认没有其他引用，一并移除 `com.pgvector:pgvector` 依赖。
- 不新增 `PGvectorTypeHandler`。如果未来出现大量基于实体的通用 CRUD，再单独评估 TypeHandler。

## 7. 原子替换

解析、切分和嵌入都在事务外完成，避免长时间占用数据库事务。只有完整的新结果准备好以后才执行：

1. 物理删除该文档旧向量。
2. 删除该文档上一轮分块；由于 MVP 不保留分块历史，重处理路径使用明确的物理删除，普通人工删除接口的既有逻辑删除语义不变。
3. 批量写入新分块。
4. 批量写入新向量。
5. 更新文档状态、分块数量、策略和配置，并清除 `lastError`。

步骤 1-5 必须在一个数据库事务内。任一步失败都回滚，使上一轮成功数据继续可用。回滚结束后，再用独立状态更新记录本轮失败。

## 8. 数据库脚本

沿用 `rag` schema 和四张核心表，不增加执行记录表。需要调整：

- `embedding` 明确为 `vector(1536)`，并建议设为非空。
- 当前普通索引改为真正的 HNSW 余弦索引：

```sql
CREATE INDEX idx_kv_embedding
ON rag.t_knowledge_vector
USING hnsw (embedding vector_cosine_ops);
```

- 为活动文档增加同知识库内容唯一约束：

```sql
CREATE UNIQUE INDEX uk_knowledge_document_kb_hash_active
ON rag.t_knowledge_document (kb_id, content_hash)
WHERE deleted = 0 AND content_hash IS NOT NULL;
```

- 更新 `resources/database/rag.sql` 作为全新安装基线，并补充一个非破坏性升级脚本供已有数据库执行；升级脚本不得 DROP 四张业务表。

## 9. 接口合同

### 上传接口

- 成功：返回 `pending` 文档，`chunkCount=0`，数据库无分块和向量。
- 文件不合法或重复：返回可理解的客户端错误，且对象存储无新增文件。
- 对象上传后数据库失败：返回失败并尝试补偿删除对象。

### 分块接口

- 同步返回；成功时返回 `completed` 文档及实际分块数量。
- 处理中重复调用：立即拒绝。
- 处理失败：接口返回失败，状态接口随后可读到 `failed` 和 `metadata.lastError`。

### 状态接口

- 复用现有 `GET /api/knowledge/documents/{doc-id}/status`。
- 不新增阶段明细、执行历史或耗时字段。

## 10. 测试设计

- 上传单元测试：扩展名、大小、空文件、重复内容、跨知识库相同内容、同名不同内容、数据库失败后的对象补偿。
- 处理服务测试：只有显式调用才处理、并发 processing 拒绝、空结果失败、解析/嵌入失败保留旧数据、成功清除错误且保留其他 metadata。
- 向量存储测试：验证批量 SQL 参数、metadata 键和向量文本格式。
- PostgreSQL/pgvector 集成测试：使用明确的测试配置验证 `vector(1536)` 批量写入、HNSW 索引脚本及事务回滚；Docker 不可用时应明确跳过集成组，普通单元测试仍可运行。
- 应用上下文测试：不访问真实 OpenAI 或生产对象存储，验证不再出现 TypeHandler 启动错误。

## 11. 风险与回滚

- 对象存储与数据库无法组成真正的分布式事务；使用“上传后建记录，失败删对象”的补偿方式，删除补偿失败时记录错误日志供人工清理。
- 50MB 文件当前会读入内存以计算哈希；MVP 接受该限制，大并发流式哈希不在本次范围。
- HNSW 索引创建需要 pgvector 版本支持，升级脚本执行前应确认扩展存在。
- 回滚应用代码时，保留 `vector(1536)`、HNSW 和唯一索引不会破坏旧接口；如必须回滚唯一索引，可单独 DROP INDEX，不需要删除业务数据。

