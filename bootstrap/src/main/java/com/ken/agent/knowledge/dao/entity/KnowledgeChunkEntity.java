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
@TableName(value = "rag.t_knowledge_chunk", autoResultMap = true)
public class KnowledgeChunkEntity {

    /**
     * 分块ID
     */
    @TableId(value = "id", type = IdType.INPUT)
    private String id;

    /**
     * 知识库ID，逻辑关联 t_knowledge_base.id
     */
    @TableField("kb_id")
    private String kbId;

    /**
     * 文档ID，逻辑关联 t_knowledge_document.id
     */
    @TableField("document_id")
    private String documentId;

    /**
     * 分块序号
     */
    @TableField("chunk_index")
    private Integer chunkIndex;

    /**
     * 分块文本内容
     */
    @TableField("content")
    private String content;

    /**
     * Token数量
     */
    @TableField("token_count")
    private Integer tokenCount;

    /**
     * 字符数量
     */
    @TableField("char_count")
    private Integer charCount;

    /**
     * 原文开始位置
     */
    @TableField("start_position")
    private Integer startPosition;

    /**
     * 原文结束位置
     */
    @TableField("end_position")
    private Integer endPosition;

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
