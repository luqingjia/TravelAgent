-- =============================================================================
-- Docker 空库初始化种子脚本：插入一个演示知识库
-- -----------------------------------------------------------------------------
-- 什么时候会执行？
-- 只有 PostgreSQL 数据目录是“第一次初始化”时，官方入口脚本才会执行
-- /docker-entrypoint-initdb.d 下的 SQL。已有数据卷再次启动不会重跑。
--
-- 为什么需要它？
-- TravelAgent 上传接口要求知识库已存在。
-- 当前 MVP 没有“创建知识库”HTTP 接口，所以本地 Docker 直接种子一条记录，
-- 方便你立刻调用上传接口联调。
--
-- 注意：
-- 1) 这是演示数据，不是生产账号体系。
-- 2) 应用本身不会自动执行迁移或种子；依赖数据库首次初始化挂载。
-- =============================================================================

-- 向 rag.t_knowledge_base 插入一条 active 知识库。
-- 字段含义按业务表结构解释：
-- id            : 知识库主键，上传 URL 里会用到
-- name          : 展示名称
-- description   : 说明文字
-- type          : 业务类型，示例使用 travel
-- owner_user_id : 逻辑归属用户；Docker 演示写 docker
-- visibility    : private / public
-- status        : active 才会被 KnowledgeBaseExists 判定为可用
-- metadata      : 扩展 JSON，这里给空对象
-- deleted       : 0 表示未删除
INSERT INTO rag.t_knowledge_base (
  id,
  name,
  description,
  type,
  owner_user_id,
  visibility,
  status,
  metadata,
  deleted
) VALUES (
  -- 固定 ID，文档和 curl 示例都写死这个值，方便复制粘贴。
  'kb_demo_001',
  -- 人类可读名称。
  'Docker Demo Knowledge Base',
  -- 标明数据来源，避免以后误以为是业务真实库。
  'Seeded by docker/initdb for local compose smoke tests',
  -- 与表注释中的 travel 类型对齐。
  'travel',
  -- 演示归属人。
  'docker',
  -- 私有库即可。
  'private',
  -- 必须是 active，否则上传接口会认为知识库不存在。
  'active',
  -- 空 JSON 对象，满足 jsonb 默认语义。
  '{}'::jsonb,
  -- 未删除。
  0
)
-- 如果同 id 已经存在，什么也不做。
-- 这样重复执行（例如手工重跑脚本）不会因为主键冲突失败。
ON CONFLICT (id) DO NOTHING;
