BEGIN;

CREATE EXTENSION IF NOT EXISTS vector;
CREATE SCHEMA IF NOT EXISTS rag;

-- 先检查历史数据。发现重复内容时中止迁移，交给业务人员确认保留哪一份，脚本不会擅自删数据。
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM rag.t_knowledge_document
        WHERE deleted = 0 AND content_hash IS NOT NULL
        GROUP BY kb_id, content_hash
        HAVING COUNT(*) > 1
    ) THEN
        RAISE EXCEPTION '同一知识库存在重复内容文档，请先清理后再执行迁移';
    END IF;
END
$$;

DROP INDEX IF EXISTS rag.idx_kv_embedding;

ALTER TABLE rag.t_knowledge_vector
    ALTER COLUMN embedding TYPE vector(1536) USING embedding::vector(1536),
    ALTER COLUMN embedding SET NOT NULL;

CREATE INDEX idx_kv_embedding
    ON rag.t_knowledge_vector
    USING hnsw (embedding vector_cosine_ops);

CREATE UNIQUE INDEX IF NOT EXISTS uk_knowledge_document_kb_hash_active
    ON rag.t_knowledge_document (kb_id, content_hash)
    WHERE deleted = 0 AND content_hash IS NOT NULL;

COMMIT;
