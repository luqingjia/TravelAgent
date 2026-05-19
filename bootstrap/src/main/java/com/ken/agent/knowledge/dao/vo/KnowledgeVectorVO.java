package com.ken.agent.knowledge.dao.vo;

import com.ken.agent.knowledge.dao.entity.KnowledgeVectorEntity;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.util.Map;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class KnowledgeVectorVO {

    /**
     * 分块ID
     */
    private String id;

    /**
     * 分块文本内容
     */
    private String content;

    /**
     * 元数据
     */
    private Map<String, Object> metadata;

    /**
     * 向量
     */
    private float[] embedding;

    public static KnowledgeVectorVO fromEntity(KnowledgeVectorEntity entity) {
        if (entity == null) {
            return null;
        }
        return KnowledgeVectorVO.builder()
                .id(entity.getId())
                .content(entity.getContent())
                .metadata(entity.getMetadata())
                .embedding(entity.getEmbedding() == null ? null : entity.getEmbedding().toArray())
                .build();
    }
}
