package com.ken.agent.knowledge.dao.mapper;

import com.baomidou.mybatisplus.core.mapper.BaseMapper;
import com.ken.agent.knowledge.dao.entity.KnowledgeVectorEntity;
import org.apache.ibatis.annotations.Delete;
import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;
import org.apache.ibatis.annotations.Select;
import org.apache.ibatis.annotations.Insert;

import java.util.List;
import java.util.Map;

@Mapper
public interface KnowledgeVectorMapper extends BaseMapper<KnowledgeVectorEntity> {

    @Insert("""
            INSERT INTO rag.t_knowledge_vector (id, content, metadata, embedding)
            VALUES (#{id}, #{content}, CAST(#{metadataJson} AS jsonb), CAST(#{embedding} AS vector))
            ON CONFLICT (id) DO UPDATE SET
                content = EXCLUDED.content,
                metadata = EXCLUDED.metadata,
                embedding = EXCLUDED.embedding
            """)
    int upsertVector(@Param("id") String id,
                     @Param("content") String content,
                     @Param("metadataJson") String metadataJson,
                     @Param("embedding") String embedding);

    @Delete("""
            DELETE FROM rag.t_knowledge_vector
            WHERE metadata ->> 'documentId' = #{documentId}
            """)
    int deleteByDocumentId(@Param("documentId") String documentId);

    @Delete("""
            DELETE FROM rag.t_knowledge_vector
            WHERE id = #{chunkId}
            """)
    int deleteByChunkId(@Param("chunkId") String chunkId);

    @Select("""
            SELECT id, content, metadata, embedding::text AS embedding
            FROM rag.t_knowledge_vector
            WHERE metadata ->> 'kbId' = #{kbId}
            """)
    List<Map<String, Object>> listByKnowledgeBaseId(@Param("kbId") String kbId);
}
