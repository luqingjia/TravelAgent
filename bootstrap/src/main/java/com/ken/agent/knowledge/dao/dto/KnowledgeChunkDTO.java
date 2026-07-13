package com.ken.agent.knowledge.dao.dto;

import com.ken.agent.knowledge.dao.entity.KnowledgeChunkEntity;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.util.HashMap;
import java.util.Map;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class KnowledgeChunkDTO {

    /**
     * 分块ID
     */
    private String id;

    /**
     * 知识库ID，逻辑关联 t_knowledge_base.id
     */
    private String kbId;

    /**
     * 文档ID，逻辑关联 t_knowledge_document.id
     */
    private String documentId;

    /**
     * 分块序号
     */
    private Integer chunkIndex;

    /**
     * 分块文本内容
     */
    private String content;

    /**
     * Token数量
     */
    private Integer tokenCount;

    /**
     * 字符数量
     */
    private Integer charCount;

    /**
     * 原文开始位置
     */
    private Integer startPosition;

    /**
     * 原文结束位置
     */
    private Integer endPosition;

    /**
     * 扩展信息
     */
    @Builder.Default
    private Map<String, Object> metadata = new HashMap<>();

    public KnowledgeChunkEntity toEntity() {
        return KnowledgeChunkEntity.builder()
                .id(id)
                .kbId(kbId)
                .documentId(documentId)
                .chunkIndex(chunkIndex)
                .content(content)
                .tokenCount(tokenCount)
                .charCount(charCount)
                .startPosition(startPosition)
                .endPosition(endPosition)
                .metadata(metadata)
                .build();
    }
}
