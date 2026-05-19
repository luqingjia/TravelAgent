package com.ken.agent.knowledge.dao.dto;

import com.ken.agent.knowledge.dao.entity.KnowledgeVectorEntity;
import com.pgvector.PGvector;
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
public class KnowledgeVectorDTO {

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
    @Builder.Default
    private Map<String, Object> metadata = new HashMap<>();

    /**
     * 向量
     */
    private float[] embedding;

    public KnowledgeVectorEntity toEntity() {
        return KnowledgeVectorEntity.builder()
                .id(id)
                .content(content)
                .metadata(metadata)
                .embedding(embedding == null ? null : new PGvector(embedding))
                .build();
    }
}
