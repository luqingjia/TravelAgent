package com.ken.agent.knowledge.dao.mapper;

import com.baomidou.mybatisplus.core.mapper.BaseMapper;
import com.ken.agent.knowledge.dao.entity.KnowledgeChunkEntity;
import org.apache.ibatis.annotations.Delete;
import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;

@Mapper
public interface KnowledgeChunkMapper extends BaseMapper<KnowledgeChunkEntity> {

    /**
     * 真正删除某个文档的全部分块，专供重新切分后的原子替换流程使用。
     *
     * @param docId 文档 ID
     * @return 删除行数
     */
    @Delete("DELETE FROM rag.t_knowledge_chunk WHERE document_id = #{docId}")
    int deletePhysicallyByDocumentId(@Param("docId") String docId);
}
