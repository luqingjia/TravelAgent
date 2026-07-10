package com.ken.agent.knowledge.service;

import com.ken.agent.core.chunk.VectorChunk;

import java.util.List;

/**
 * pgvector 向量数据访问接口。
 *
 * <p>普通知识库表继续交给 MyBatis-Plus；向量列使用明确的 PostgreSQL SQL 和类型转换写入。
 * 实现使用与 MyBatis 相同的数据源，因此被上层事务调用时，向量增删也会一起提交或回滚。</p>
 */
public interface KnowledgeVectorService {

    /** 当前嵌入模型和数据库向量列共同约定的维度。 */
    int EMBEDDING_DIMENSIONS = 1536;

    /**
     * 批量新增或覆盖一个文档的分块向量。
     *
     * @param kbId 知识库 ID，会写入向量元数据
     * @param documentId 文档 ID，会写入向量元数据并用于后续按文档删除
     * @param chunks 已完成向量生成的分块；空列表直接返回
     * @throws com.ken.agent.framework.exception.ServiceException 任一分块缺少向量或维度不是 1536 时抛出
     */
    void indexDocumentChunks(String kbId, String documentId, List<VectorChunk> chunks);

    /**
     * 新增或覆盖单个分块向量。
     *
     * @param kbId 知识库 ID
     * @param documentId 文档 ID
     * @param chunk 已完成向量生成的分块
     * @throws com.ken.agent.framework.exception.ServiceException 分块向量为空或维度不正确时抛出
     */
    void upsertChunk(String kbId, String documentId, VectorChunk chunk);

    /**
     * 删除一个文档的全部向量。
     *
     * @param documentId 文档 ID；为空时直接返回
     */
    void deleteDocumentVectors(String documentId);

    /**
     * 删除单个分块向量。
     *
     * @param chunkId 分块 ID；为空时直接返回
     */
    void deleteChunkVector(String chunkId);
}
