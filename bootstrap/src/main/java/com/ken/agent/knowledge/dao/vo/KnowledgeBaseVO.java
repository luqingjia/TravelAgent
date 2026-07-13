package com.ken.agent.knowledge.dao.vo;

import com.ken.agent.knowledge.dao.entity.KnowledgeBaseEntity;
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
public class KnowledgeBaseVO {

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
    private Map<String, Object> metadata;

    /**
     * 创建时间
     */
    private Date createTime;

    /**
     * 更新时间
     */
    private Date updateTime;

    public static KnowledgeBaseVO fromEntity(KnowledgeBaseEntity entity) {
        if (entity == null) {
            return null;
        }
        return KnowledgeBaseVO.builder()
                .id(entity.getId())
                .name(entity.getName())
                .description(entity.getDescription())
                .type(entity.getType())
                .ownerUserId(entity.getOwnerUserId())
                .visibility(entity.getVisibility())
                .status(entity.getStatus())
                .metadata(entity.getMetadata())
                .createTime(entity.getCreateTime())
                .updateTime(entity.getUpdateTime())
                .build();
    }
}
