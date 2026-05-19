package com.ken.agent.knowledge.dao.entity;

import com.baomidou.mybatisplus.annotation.IdType;
import com.baomidou.mybatisplus.annotation.TableField;
import com.baomidou.mybatisplus.annotation.TableId;
import com.baomidou.mybatisplus.annotation.TableName;
import com.baomidou.mybatisplus.extension.handlers.JacksonTypeHandler;
import com.pgvector.PGvector;
import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

import java.util.Map;

@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
@TableName(value = "rag.t_knowledge_vector", autoResultMap = true)
public class KnowledgeVectorEntity {

    /**
     * 分块ID
     */
    @TableId(value = "id", type = IdType.INPUT)
    private String id;

    /**
     * 分块文本内容
     */
    @TableField("content")
    private String content;

    /**
     * 元数据
     */
    @TableField(value = "metadata", typeHandler = JacksonTypeHandler.class)
    private Map<String, Object> metadata;

    /**
     * 向量
     */
    @TableField("embedding")
    private PGvector embedding;
}
