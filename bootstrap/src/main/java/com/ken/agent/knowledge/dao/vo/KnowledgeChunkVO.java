package com.ken.agent.knowledge.dao.vo;

import com.ken.agent.knowledge.dao.entity.KnowledgeChunkEntity;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.util.Date;
import java.util.Map;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class KnowledgeChunkVO {

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
    private Map<String, Object> metadata;

    /**
     * 创建时间
     */
    private Date createTime;

    /**
     * 更新时间
     */
    private Date updateTime;

    public static KnowledgeChunkVO fromEntity(KnowledgeChunkEntity entity) {
        if (entity == null) {
            return null;
        }
        return KnowledgeChunkVO.builder()
                .id(entity.getId())
                .kbId(entity.getKbId())
                .documentId(entity.getDocumentId())
                .chunkIndex(entity.getChunkIndex())
                .content(entity.getContent())
                .tokenCount(entity.getTokenCount())
                .charCount(entity.getCharCount())
                .startPosition(entity.getStartPosition())
                .endPosition(entity.getEndPosition())
                .metadata(entity.getMetadata())
                .createTime(entity.getCreateTime())
                .updateTime(entity.getUpdateTime())
                .build();
    }
}
