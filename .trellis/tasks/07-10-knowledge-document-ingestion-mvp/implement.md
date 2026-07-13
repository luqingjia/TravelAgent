# 知识库文档摄取 MVP 实施计划

## 实施顺序

### 1. 固化基线与测试隔离

- [ ] 运行根项目 `mvn test`，保存当前 `No typehandler found for property embedding` 失败证据。
- [ ] 梳理现有 `AgentApplicationTests`，把真实 OpenAI、生产数据库和对象存储依赖替换为显式测试配置或 Mock。
- [ ] 为上传、处理状态和向量存储建立聚焦测试骨架，先写失败用例再修改实现。

### 2. 修正数据库基线与升级脚本

- [ ] 更新 `resources/database/rag.sql`：`vector(1536)`、HNSW 余弦索引、活动文档内容唯一索引。
- [ ] 新增非破坏性升级 SQL，覆盖已有表的列类型和索引调整，不 DROP 业务表。
- [ ] 为分块 Mapper 增加仅供重新处理使用的按文档物理删除方法。
- [ ] 验证重复数据存在时升级脚本给出明确失败，不静默删除用户数据。

### 3. 增加上传策略配置

- [ ] 在 `bootstrap` 知识库业务包新增 `KnowledgeDocumentUploadProperties`。
- [ ] 在 `application.yml` 增加允许扩展名和最大文件大小配置，默认值与 PRD 一致，并提供环境变量覆盖。
- [ ] 增加大小写不敏感的扩展名规范化与 `DataSize` 大小校验。
- [ ] 增加配置绑定测试，验证修改配置后服务使用新值。

### 4. 完成上传前校验与去重

- [ ] 调整 `KnowledgeDocumentServiceImpl.upload` 顺序：文件策略校验 -> 读取与哈希 -> 重复查询 -> 对象上传 -> 文档保存。
- [ ] 增加同知识库活动文档的 `contentHash` 查询。
- [ ] 将数据库唯一冲突转换为明确的重复文档错误。
- [ ] 文档保存失败时补偿删除本次对象；补偿失败只记录日志，不覆盖原始异常。
- [ ] 验证上传成功保持 `pending`，且不会隐式调用分块处理。

### 5. 将向量存储改为 ragent 风格

- [ ] 保留 `KnowledgeVectorService` 接口，将实现改为构造器注入 `JdbcTemplate` 和 `ObjectMapper`。
- [ ] 使用 `batchUpdate` 实现 `rag.t_knowledge_vector` 批量 upsert，SQL 显式转换 `jsonb` 和 `vector`。
- [ ] 实现按文档和按分块的物理删除，并保持 TravelAgent 现有 metadata 键。
- [ ] 统一校验空向量和 1536 维度，统一转换 `float[]` 为 PostgreSQL vector 文本。
- [ ] 移除向量 MyBatis Mapper、无调用的 PGvector 实体转换代码及确认无引用后的 pgvector Java 依赖。
- [ ] 运行应用上下文测试，确认 TypeHandler 错误消失。

### 6. 加固同步处理、状态与原子替换

- [ ] 将进入 `processing` 改为数据库条件更新，保证同一文档只有一个请求成功进入。
- [ ] 在替换前校验解析文本、分块列表和全部嵌入结果非空且维度正确。
- [ ] 在现有 `TransactionOperations` 中执行旧数据删除、新分块写入、新向量写入和成功状态更新。
- [ ] 成功更新合并原 metadata 并删除 `lastError`；失败更新合并原 metadata 并覆盖最近错误。
- [ ] 验证解析或嵌入失败不触碰旧分块/向量，持久化失败通过事务回滚恢复旧数据。

### 7. 完成自动化验证

- [ ] 为知识库服务接口和全部 `*ServiceImpl` 补齐 Javadoc，并为上传、重试、原子替换、向量批处理等复杂流程补充大白话注释。
- [ ] 上传测试覆盖所有允许/拒绝、去重和补偿场景。
- [ ] 处理测试覆盖首次处理、重新处理、并发拒绝、失败保留、成功替换和错误清除。
- [ ] 向量测试覆盖批量参数、事务参与、1536 维度和 PostgreSQL 实际写入。
- [ ] 运行 `mvn test`。
- [ ] 运行 `mvn clean package`。
- [ ] 检查 `git diff --check`、`git status --short`，确认没有密钥、`.env`、IDE 文件或 Trellis 运行文件被纳入业务提交。

## 重点文件

- `bootstrap/src/main/java/com/ken/agent/knowledge/service/impl/KnowledgeDocumentServiceImpl.java`
- `bootstrap/src/main/java/com/ken/agent/knowledge/service/impl/KnowledgeVectorServiceImpl.java`
- `bootstrap/src/main/java/com/ken/agent/knowledge/service/KnowledgeVectorService.java`
- `bootstrap/src/main/java/com/ken/agent/knowledge/dao/mapper/KnowledgeChunkMapper.java`
- `bootstrap/src/main/java/com/ken/agent/knowledge/dao/mapper/KnowledgeDocumentMapper.java`
- `bootstrap/src/main/resources/application.yml`
- `resources/database/rag.sql`
- `bootstrap/src/test/java/com/ken/agent/**`

## 高风险点与回滚点

- **数据库唯一索引**：上线前先查重；发现历史重复数据时停止迁移并人工处理，禁止脚本自动删除。
- **向量批量 SQL**：先用集成测试验证 `jsonb/vector` 参数转换，再删除旧 Mapper；需要回滚时可恢复旧自定义 SQL，但不要恢复会触发启动错误的 `BaseMapper<KnowledgeVectorEntity>`。
- **混合持久化事务**：必须确认 MyBatis-Plus 与 `JdbcTemplate` 使用同一 DataSource；通过故意让向量写入失败的测试证明旧分块未被提交删除。
- **对象存储补偿**：补偿删除失败不能覆盖数据库异常，日志必须包含可定位的对象地址但不得包含密钥。
- **元数据更新**：所有状态变化都基于旧 metadata 合并，避免丢失对象存储信息。

## 开始实现前检查

- [x] 用户已评审并批准 `prd.md`、`design.md`、`implement.md`。
- [x] 运行 `trellis-before-dev` 读取当前后端规范和预开发检查项。
- [x] 已执行 `task.py start`，任务状态为 `in_progress`。
