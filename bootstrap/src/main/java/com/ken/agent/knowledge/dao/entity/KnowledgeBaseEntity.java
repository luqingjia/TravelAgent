package com.ken.agent.knowledge.dao.entity;

import com.baomidou.mybatisplus.annotation.FieldFill;
import com.baomidou.mybatisplus.annotation.IdType;
import com.baomidou.mybatisplus.annotation.TableField;
import com.baomidou.mybatisplus.annotation.TableId;
import com.baomidou.mybatisplus.annotation.TableLogic;
import com.baomidou.mybatisplus.annotation.TableName;
import com.baomidou.mybatisplus.extension.handlers.JacksonTypeHandler;
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
@TableName(value = "rag.t_knowledge_base", autoResultMap = true)
public class KnowledgeBaseEntity {

    /**
     * 知识库ID
     */
    @TableId(value = "id", type = IdType.INPUT)
    private String id;

    /**
     * 知识库名称
     */
    @TableField("name")
    private String name;

    /**
     * 知识库描述
     */
    @TableField("description")
    private String description;

    /**
     * 知识库类型：travel/hotel/visa/traffic
     */
    @TableField("type")
    private String type;

    /**
     * 所属用户ID，逻辑关联用户表ID
     */
    @TableField("owner_user_id")
    private String ownerUserId;

    /**
     * 可见性：private/public
     */
    @TableField("visibility")
    private String visibility;

    /**
     * 状态：active/disabled
     */
    @TableField("status")
    private String status;

    /**
     * 扩展信息
     */
    @TableField(value = "metadata", typeHandler = JacksonTypeHandler.class)
    private Map<String, Object> metadata;

    /**
     * 创建时间
     */
    @TableField(value = "create_time", fill = FieldFill.INSERT)
    private Date createTime;

    /**
     * 更新时间
     */
    @TableField(value = "update_time", fill = FieldFill.INSERT_UPDATE)
    private Date updateTime;

    /**
     * 逻辑删除：0未删除，1已删除
     */
    @TableLogic(value = "0", delval = "1")
    @TableField(value = "deleted", fill = FieldFill.INSERT)
    private Short deleted;
}
