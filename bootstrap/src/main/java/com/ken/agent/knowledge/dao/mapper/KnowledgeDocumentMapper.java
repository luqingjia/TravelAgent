package com.ken.agent.knowledge.dao.mapper;

import com.baomidou.mybatisplus.core.mapper.BaseMapper;
import com.ken.agent.knowledge.dao.entity.KnowledgeDocumentEntity;
import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;
import org.apache.ibatis.annotations.Select;
import org.apache.ibatis.annotations.Update;

@Mapper
public interface KnowledgeDocumentMapper extends BaseMapper<KnowledgeDocumentEntity> {

    /**
     * 判断同一知识库是否已有内容指纹相同的有效文档。
     *
     * @param kbId 知识库 ID
     * @param contentHash 文件内容 SHA-256
     * @return 存在重复内容时返回 {@code true}
     */
    @Select("""
            SELECT EXISTS (
                SELECT 1
                FROM rag.t_knowledge_document
                WHERE kb_id = #{kbId}
                  AND content_hash = #{contentHash}
                  AND deleted = 0
            )
            """)
    boolean existsActiveByKbIdAndContentHash(@Param("kbId") String kbId,
                                             @Param("contentHash") String contentHash);

    /**
     * 原子地把未在处理中的文档改为处理中。
     *
     * @param docId 文档 ID
     * @return 更新行数；1 表示取得处理权，0 表示文档不存在或已有请求正在处理
     */
    @Update("""
            UPDATE rag.t_knowledge_document
            SET status = 'processing', update_time = CURRENT_TIMESTAMP
            WHERE id = #{docId}
              AND deleted = 0
              AND status <> 'processing'
            """)
    int tryMarkProcessing(@Param("docId") String docId);
}
