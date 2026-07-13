package com.ken.agent.knowledge.dao.dto;

import com.ken.agent.knowledge.dao.entity.KnowledgeBaseEntity;
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
public class KnowledgeBaseDTO {

    /**
     * 知识库ID
     */
    private String id;

    /**
     * 知识库名称
     */
    private String name;

    /**
     * 知识库描述
     */
    private String description;

    /**
     * 知识库类型：travel/hotel/visa/traffic
     */
    private String type;

    /**
     * 所属用户ID，逻辑关联用户表ID
     */
    private String ownerUserId;

    /**
     * 可见性：private/public
     */
    private String visibility;

    /**
     * 状态：active/disabled
     */
    private String status;

    /**
     * 扩展信息
     */
    @Builder.Default
    private Map<String, Object> metadata = new HashMap<>();

    public KnowledgeBaseEntity toEntity() {
        return KnowledgeBaseEntity.builder()
                .id(id)
                .name(name)
                .description(description)
                .type(type)
                .ownerUserId(ownerUserId)
                .visibility(visibility)
                .status(status)
                .metadata(metadata)
                .build();
    }
}
