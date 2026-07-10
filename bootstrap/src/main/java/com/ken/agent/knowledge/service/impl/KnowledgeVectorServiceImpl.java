package com.ken.agent.knowledge.service.impl;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.ken.agent.core.chunk.VectorChunk;
import com.ken.agent.framework.errorcode.BaseErrorCode;
import com.ken.agent.framework.exception.ServiceException;
import com.ken.agent.knowledge.service.KnowledgeVectorService;
import lombok.RequiredArgsConstructor;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Service;

import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * pgvector 向量读写的具体实现。
 *
 * <p>普通表适合用 MyBatis-Plus，但 PostgreSQL 的 {@code vector} 类型需要显式写出
 * {@code ?::vector}。这里借鉴 ragent 的做法，用 {@link JdbcTemplate} 直接执行少量、固定的 SQL。
 * 这不是另一套数据库连接：JdbcTemplate 会复用 Spring 当前事务绑定的数据源连接，因此上层替换
 * 分块失败时，这里的向量删除和新增也会一起回滚。</p>
 */
@Service
@RequiredArgsConstructor
public class KnowledgeVectorServiceImpl implements KnowledgeVectorService {

    private static final String UPSERT_SQL = """
            INSERT INTO rag.t_knowledge_vector (id, content, metadata, embedding)
            VALUES (?, ?, ?::jsonb, ?::vector)
            ON CONFLICT (id) DO UPDATE SET
                content = EXCLUDED.content,
                metadata = EXCLUDED.metadata,
                embedding = EXCLUDED.embedding
            """;
    private static final String DELETE_DOCUMENT_SQL =
            "DELETE FROM rag.t_knowledge_vector WHERE metadata ->> 'documentId' = ?";
    private static final String DELETE_CHUNK_SQL =
            "DELETE FROM rag.t_knowledge_vector WHERE id = ?";

    private final JdbcTemplate jdbcTemplate;
    private final ObjectMapper objectMapper;

    /**
     * {@inheritDoc}
     *
     * <p>先把所有分块检查完，再发出一次 JDBC 批量请求。这样不会写到一半才发现后面的向量维度错误。</p>
     */
    @Override
    public void indexDocumentChunks(String kbId, String documentId, List<VectorChunk> chunks) {
        if (chunks == null || chunks.isEmpty()) {
            return;
        }
        for (VectorChunk chunk : chunks) {
            validateChunk(chunk);
        }
        // 一批分块只和数据库往返一次；文档较大时会比逐条 insert 明显省时间。
        jdbcTemplate.batchUpdate(
                UPSERT_SQL,
                chunks,
                chunks.size(),
                (statement, chunk) -> setStatementValues(
                        statement, kbId, documentId, chunk));
    }

    /**
     * {@inheritDoc}
     *
     * <p>SQL 使用分块 ID 冲突更新，所以既能新增，也能覆盖手工修改前的旧向量。</p>
     */
    @Override
    public void upsertChunk(String kbId, String documentId, VectorChunk chunk) {
        validateChunk(chunk);
        jdbcTemplate.update(
                UPSERT_SQL,
                chunk.getChunkId(),
                chunk.getContent(),
                toJson(buildMetadata(kbId, documentId, chunk)),
                toPgVector(chunk.getEmbedding()));
    }

    /** {@inheritDoc} */
    @Override
    public void deleteDocumentVectors(String documentId) {
        if (documentId == null || documentId.isBlank()) {
            return;
        }
        jdbcTemplate.update(DELETE_DOCUMENT_SQL, documentId);
    }

    /** {@inheritDoc} */
    @Override
    public void deleteChunkVector(String chunkId) {
        if (chunkId == null || chunkId.isBlank()) {
            return;
        }
        jdbcTemplate.update(DELETE_CHUNK_SQL, chunkId);
    }

    /**
     * 为 JDBC 批量写入中的一条分块设置四个占位参数。
     *
     * @param statement 当前批次复用的预编译语句
     * @param kbId 知识库 ID
     * @param documentId 文档 ID
     * @param chunk 当前分块
     * @throws java.sql.SQLException JDBC 设置参数失败时抛出，交给 Spring 统一转换并触发事务回滚
     */
    private void setStatementValues(java.sql.PreparedStatement statement,
                                    String kbId,
                                    String documentId,
                                    VectorChunk chunk) throws java.sql.SQLException {
        statement.setString(1, chunk.getChunkId());
        statement.setString(2, chunk.getContent());
        statement.setString(3, toJson(buildMetadata(kbId, documentId, chunk)));
        statement.setString(4, toPgVector(chunk.getEmbedding()));
    }

    /**
     * 在接触数据库前检查向量是否可写。
     *
     * @param chunk 待写入分块
     * @throws ServiceException 分块为空、没有向量或维度与数据库约定不一致时抛出
     */
    private void validateChunk(VectorChunk chunk) {
        if (chunk == null || chunk.getEmbedding() == null || chunk.getEmbedding().length == 0) {
            throw new ServiceException("Chunk 向量不能为空");
        }
        if (chunk.getEmbedding().length != KnowledgeVectorService.EMBEDDING_DIMENSIONS) {
            throw new ServiceException("Chunk 向量维度必须为 " + KnowledgeVectorService.EMBEDDING_DIMENSIONS);
        }
    }

    /**
     * 合并业务元数据和检索所需的固定定位信息。
     *
     * <p>固定字段最后写入，防止调用方元数据里同名字段把真实知识库、文档或分块 ID 覆盖掉。</p>
     *
     * @param kbId 知识库 ID
     * @param documentId 文档 ID
     * @param chunk 当前分块
     * @return 可序列化为 JSONB 的元数据
     */
    private Map<String, Object> buildMetadata(String kbId, String documentId, VectorChunk chunk) {
        Map<String, Object> metadata = new HashMap<>();
        if (chunk.getMetadata() != null) {
            metadata.putAll(chunk.getMetadata());
        }
        metadata.put("kbId", kbId);
        metadata.put("documentId", documentId);
        metadata.put("chunkId", chunk.getChunkId());
        metadata.put("chunkIndex", chunk.getIndex());
        return metadata;
    }

    /**
     * 把元数据转换成 PostgreSQL JSONB 能接收的 JSON 字符串。
     *
     * @param metadata 元数据
     * @return JSON 字符串
     * @throws ServiceException 元数据中包含无法序列化的对象时抛出
     */
    private String toJson(Map<String, Object> metadata) {
        try {
            return objectMapper.writeValueAsString(metadata);
        } catch (JsonProcessingException ex) {
            throw new ServiceException("向量元数据序列化失败", ex, BaseErrorCode.SERVICE_ERROR);
        }
    }

    /**
     * 把 Java 浮点数组转换成 pgvector 接受的 {@code [0.1,0.2,...]} 文本格式。
     *
     * <p>SQL 中的 {@code ?::vector} 会让 PostgreSQL 完成最终类型转换，因此不依赖第三方
     * PGvector TypeHandler，也不会再经过 MyBatis-Plus 的对象映射。</p>
     *
     * @param embedding 维度已经校验过的向量
     * @return pgvector 文本表示
     */
    private String toPgVector(float[] embedding) {
        StringBuilder builder = new StringBuilder("[");
        for (int i = 0; i < embedding.length; i++) {
            if (i > 0) {
                builder.append(',');
            }
            builder.append(embedding[i]);
        }
        return builder.append(']').toString();
    }
}
