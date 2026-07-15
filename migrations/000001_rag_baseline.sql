-- 仅用于全新空数据库初始化。
-- 本文件包含重建语句，禁止在已有业务数据的数据库上直接执行。

-- 本脚本由历史 PostgreSQL schema 整理而来。
-- 已移除导出工具中的来源主机、库名等本地环境信息，只保留建表语义。

CREATE EXTENSION IF NOT EXISTS vector;
CREATE SCHEMA IF NOT EXISTS rag;
-- ----------------------------
-- Table structure for t_knowledge_base
-- ----------------------------
DROP TABLE IF EXISTS "rag"."t_knowledge_base";
CREATE TABLE "rag"."t_knowledge_base" (
  "id" varchar(20) COLLATE "pg_catalog"."default" NOT NULL,
  "name" varchar(128) COLLATE "pg_catalog"."default" NOT NULL,
  "description" text COLLATE "pg_catalog"."default",
  "type" varchar(32) COLLATE "pg_catalog"."default" DEFAULT 'travel'::character varying,
  "owner_user_id" varchar(20) COLLATE "pg_catalog"."default",
  "visibility" varchar(16) COLLATE "pg_catalog"."default" DEFAULT 'private'::character varying,
  "status" varchar(16) COLLATE "pg_catalog"."default" DEFAULT 'active'::character varying,
  "metadata" jsonb DEFAULT '{}'::jsonb,
  "create_time" timestamp(6) DEFAULT CURRENT_TIMESTAMP,
  "update_time" timestamp(6) DEFAULT CURRENT_TIMESTAMP,
  "deleted" int2 DEFAULT 0
)
;
COMMENT ON COLUMN "rag"."t_knowledge_base"."id" IS '知识库ID';
COMMENT ON COLUMN "rag"."t_knowledge_base"."name" IS '知识库名称';
COMMENT ON COLUMN "rag"."t_knowledge_base"."description" IS '知识库描述';
COMMENT ON COLUMN "rag"."t_knowledge_base"."type" IS '知识库类型：travel/hotel/visa/traffic';
COMMENT ON COLUMN "rag"."t_knowledge_base"."owner_user_id" IS '所属用户ID，逻辑关联用户表ID';
COMMENT ON COLUMN "rag"."t_knowledge_base"."visibility" IS '可见性：private/public';
COMMENT ON COLUMN "rag"."t_knowledge_base"."status" IS '状态：active/disabled';
COMMENT ON COLUMN "rag"."t_knowledge_base"."metadata" IS '扩展信息';
COMMENT ON COLUMN "rag"."t_knowledge_base"."deleted" IS '逻辑删除：0未删除，1已删除';
COMMENT ON TABLE "rag"."t_knowledge_base" IS '知识库表';

-- ----------------------------
-- Table structure for t_knowledge_chunk
-- ----------------------------
DROP TABLE IF EXISTS "rag"."t_knowledge_chunk";
CREATE TABLE "rag"."t_knowledge_chunk" (
  "id" varchar(20) COLLATE "pg_catalog"."default" NOT NULL,
  "kb_id" varchar(20) COLLATE "pg_catalog"."default" NOT NULL,
  "document_id" varchar(20) COLLATE "pg_catalog"."default" NOT NULL,
  "chunk_index" int4 NOT NULL,
  "content" text COLLATE "pg_catalog"."default" NOT NULL,
  "token_count" int4,
  "char_count" int4,
  "start_position" int4,
  "end_position" int4,
  "metadata" jsonb DEFAULT '{}'::jsonb,
  "enabled" int2 DEFAULT 1,
  "create_time" timestamp(6) DEFAULT CURRENT_TIMESTAMP,
  "update_time" timestamp(6) DEFAULT CURRENT_TIMESTAMP,
  "deleted" int2 DEFAULT 0
)
;
COMMENT ON COLUMN "rag"."t_knowledge_chunk"."id" IS '分块ID';
COMMENT ON COLUMN "rag"."t_knowledge_chunk"."kb_id" IS '知识库ID，逻辑关联 t_knowledge_base.id';
COMMENT ON COLUMN "rag"."t_knowledge_chunk"."document_id" IS '文档ID，逻辑关联 t_knowledge_document.id';
COMMENT ON COLUMN "rag"."t_knowledge_chunk"."chunk_index" IS '分块序号';
COMMENT ON COLUMN "rag"."t_knowledge_chunk"."content" IS '分块文本内容';
COMMENT ON COLUMN "rag"."t_knowledge_chunk"."token_count" IS 'Token数量';
COMMENT ON COLUMN "rag"."t_knowledge_chunk"."char_count" IS '字符数量';
COMMENT ON COLUMN "rag"."t_knowledge_chunk"."start_position" IS '原文开始位置';
COMMENT ON COLUMN "rag"."t_knowledge_chunk"."end_position" IS '原文结束位置';
COMMENT ON COLUMN "rag"."t_knowledge_chunk"."metadata" IS '扩展信息';
COMMENT ON COLUMN "rag"."t_knowledge_chunk"."enabled" IS '是否启用：1启用，0禁用';
COMMENT ON COLUMN "rag"."t_knowledge_chunk"."deleted" IS '逻辑删除：0未删除，1已删除';
COMMENT ON TABLE "rag"."t_knowledge_chunk" IS '知识库文档分块表';

-- ----------------------------
-- Table structure for t_knowledge_document
-- ----------------------------
DROP TABLE IF EXISTS "rag"."t_knowledge_document";
CREATE TABLE "rag"."t_knowledge_document" (
  "id" varchar(20) COLLATE "pg_catalog"."default" NOT NULL,
  "kb_id" varchar(20) COLLATE "pg_catalog"."default" NOT NULL,
  "title" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "source_type" varchar(32) COLLATE "pg_catalog"."default" NOT NULL,
  "source_uri" text COLLATE "pg_catalog"."default",
  "file_name" varchar(255) COLLATE "pg_catalog"."default",
  "file_type" varchar(32) COLLATE "pg_catalog"."default",
  "file_size" int8,
  "content_hash" varchar(128) COLLATE "pg_catalog"."default",
  "language" varchar(16) COLLATE "pg_catalog"."default" DEFAULT 'zh'::character varying,
  "status" varchar(32) COLLATE "pg_catalog"."default" DEFAULT 'pending'::character varying,
  "chunk_count" int4 DEFAULT 0,
  "chunk_strategy" varchar(64) COLLATE "pg_catalog"."default" DEFAULT 'structure_aware'::character varying,
  "chunk_config" jsonb DEFAULT '{}'::jsonb,
  "metadata" jsonb DEFAULT '{}'::jsonb,
  "create_time" timestamp(6) DEFAULT CURRENT_TIMESTAMP,
  "update_time" timestamp(6) DEFAULT CURRENT_TIMESTAMP,
  "deleted" int2 DEFAULT 0
)
;
COMMENT ON COLUMN "rag"."t_knowledge_document"."id" IS '文档ID';
COMMENT ON COLUMN "rag"."t_knowledge_document"."kb_id" IS '知识库ID，逻辑关联 t_knowledge_base.id';
COMMENT ON COLUMN "rag"."t_knowledge_document"."title" IS '文档标题';
COMMENT ON COLUMN "rag"."t_knowledge_document"."source_type" IS '来源类型：file/url/manual/api';
COMMENT ON COLUMN "rag"."t_knowledge_document"."source_uri" IS '来源地址，如文件路径或网页URL';
COMMENT ON COLUMN "rag"."t_knowledge_document"."file_name" IS '文件名';
COMMENT ON COLUMN "rag"."t_knowledge_document"."file_type" IS '文件类型，如 pdf/txt/md/docx/html';
COMMENT ON COLUMN "rag"."t_knowledge_document"."file_size" IS '文件大小，单位字节';
COMMENT ON COLUMN "rag"."t_knowledge_document"."content_hash" IS '内容哈希，用于去重';
COMMENT ON COLUMN "rag"."t_knowledge_document"."language" IS '语言';
COMMENT ON COLUMN "rag"."t_knowledge_document"."status" IS '处理状态：pending/processing/completed/failed';
COMMENT ON COLUMN "rag"."t_knowledge_document"."chunk_count" IS '分块数量';
COMMENT ON COLUMN "rag"."t_knowledge_document"."chunk_strategy" IS '分块策略';
COMMENT ON COLUMN "rag"."t_knowledge_document"."chunk_config" IS '分块参数配置';
COMMENT ON COLUMN "rag"."t_knowledge_document"."metadata" IS '扩展信息';
COMMENT ON COLUMN "rag"."t_knowledge_document"."deleted" IS '逻辑删除：0未删除，1已删除';
COMMENT ON TABLE "rag"."t_knowledge_document" IS '知识库文档表';

-- ----------------------------
-- Table structure for t_knowledge_vector
-- ----------------------------
DROP TABLE IF EXISTS "rag"."t_knowledge_vector";
CREATE TABLE "rag"."t_knowledge_vector" (
  "id" varchar(20) COLLATE "pg_catalog"."default" NOT NULL,
  "content" text COLLATE "pg_catalog"."default",
  "metadata" jsonb,
  "embedding" vector(1536) NOT NULL
)
;
COMMENT ON COLUMN "rag"."t_knowledge_vector"."id" IS '分块ID';
COMMENT ON COLUMN "rag"."t_knowledge_vector"."content" IS '分块文本内容';
COMMENT ON COLUMN "rag"."t_knowledge_vector"."metadata" IS '元数据';
COMMENT ON COLUMN "rag"."t_knowledge_vector"."embedding" IS '向量';
COMMENT ON TABLE "rag"."t_knowledge_vector" IS '知识库向量存储表';

-- ----------------------------
-- Indexes structure for table t_knowledge_base
-- ----------------------------
CREATE INDEX "idx_knowledge_base_deleted" ON "rag"."t_knowledge_base" USING btree (
  "deleted" "pg_catalog"."int2_ops" ASC NULLS LAST
);
CREATE INDEX "idx_knowledge_base_owner" ON "rag"."t_knowledge_base" USING btree (
  "owner_user_id" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
);
CREATE INDEX "idx_knowledge_base_status" ON "rag"."t_knowledge_base" USING btree (
  "status" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
);

-- ----------------------------
-- Primary Key structure for table t_knowledge_base
-- ----------------------------
ALTER TABLE "rag"."t_knowledge_base" ADD CONSTRAINT "t_knowledge_base_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table t_knowledge_chunk
-- ----------------------------
CREATE INDEX "idx_knowledge_chunk_deleted" ON "rag"."t_knowledge_chunk" USING btree (
  "deleted" "pg_catalog"."int2_ops" ASC NULLS LAST
);
CREATE INDEX "idx_knowledge_chunk_document" ON "rag"."t_knowledge_chunk" USING btree (
  "document_id" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
);
CREATE INDEX "idx_knowledge_chunk_kb" ON "rag"."t_knowledge_chunk" USING btree (
  "kb_id" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
);
CREATE INDEX "idx_knowledge_chunk_enabled" ON "rag"."t_knowledge_chunk" USING btree (
  "enabled" "pg_catalog"."int2_ops" ASC NULLS LAST
);
CREATE INDEX "idx_knowledge_chunk_order" ON "rag"."t_knowledge_chunk" USING btree (
  "document_id" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST,
  "chunk_index" "pg_catalog"."int4_ops" ASC NULLS LAST
);

-- ----------------------------
-- Primary Key structure for table t_knowledge_chunk
-- ----------------------------
ALTER TABLE "rag"."t_knowledge_chunk" ADD CONSTRAINT "t_knowledge_chunk_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table t_knowledge_document
-- ----------------------------
CREATE INDEX "idx_knowledge_document_deleted" ON "rag"."t_knowledge_document" USING btree (
  "deleted" "pg_catalog"."int2_ops" ASC NULLS LAST
);
CREATE INDEX "idx_knowledge_document_hash" ON "rag"."t_knowledge_document" USING btree (
  "content_hash" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
);
CREATE INDEX "idx_knowledge_document_kb" ON "rag"."t_knowledge_document" USING btree (
  "kb_id" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
);
CREATE INDEX "idx_knowledge_document_status" ON "rag"."t_knowledge_document" USING btree (
  "status" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
);
CREATE UNIQUE INDEX "uk_knowledge_document_kb_hash_active" ON "rag"."t_knowledge_document" ("kb_id", "content_hash") WHERE deleted = 0 AND content_hash IS NOT NULL;

-- ----------------------------
-- Primary Key structure for table t_knowledge_document
-- ----------------------------
ALTER TABLE "rag"."t_knowledge_document" ADD CONSTRAINT "t_knowledge_document_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table t_knowledge_vector
-- ----------------------------
CREATE INDEX "idx_kv_embedding" ON "rag"."t_knowledge_vector" USING hnsw (embedding vector_cosine_ops);
CREATE INDEX "idx_kv_metadata" ON "rag"."t_knowledge_vector" USING gin (
  "metadata" "pg_catalog"."jsonb_ops"
);

-- ----------------------------
-- Primary Key structure for table t_knowledge_vector
-- ----------------------------
ALTER TABLE "rag"."t_knowledge_vector" ADD CONSTRAINT "t_knowledge_vector_pkey" PRIMARY KEY ("id");
