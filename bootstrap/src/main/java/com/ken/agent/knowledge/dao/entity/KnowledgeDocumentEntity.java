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
@TableName(value = "rag.t_knowledge_document", autoResultMap = true)
public class KnowledgeDocumentEntity {

    /**
     * 文档ID
     */
    @TableId(value = "id", type = IdType.INPUT)
    private String id;

    /**
     * 知识库ID，逻辑关联 t_knowledge_base.id
     */
    @TableField("kb_id")
    private String kbId;

    /**
     * 文档标题
     */
    @TableField("title")
    private String title;

    /**
     * 来源类型：file/url/manual/api
     */
    @TableField("source_type")
    private String sourceType;

    /**
     * 来源地址，如文件路径或网页URL
     */
    @TableField("source_uri")
    private String sourceUri;

    /**
     * 文件名
     */
    @TableField("file_name")
    private String fileName;

    /**
     * 文件类型，如 pdf/txt/md/docx/html
     */
    @TableField("file_type")
    private String fileType;

    /**
     * 文件大小，单位字节
     */
    @TableField("file_size")
    private Long fileSize;

    /**
     * 内容哈希，用于去重
     */
    @TableField("content_hash")
    private String contentHash;

    /**
     * 语言
     */
    @TableField("language")
    private String language;

    /**
     * 处理状态：pending/processing/completed/failed
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
