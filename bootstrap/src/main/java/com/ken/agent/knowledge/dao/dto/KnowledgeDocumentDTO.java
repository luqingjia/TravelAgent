package com.ken.agent.knowledge.dao.dto;

import com.ken.agent.knowledge.dao.entity.KnowledgeDocumentEntity;
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
public class KnowledgeDocumentDTO {

    /**
     * 文档ID
     */
    private String id;

    /**
     * 知识库ID，逻辑关联 t_knowledge_base.id
     */
    private String kbId;

    /**
     * 文档标题
     */
    private String title;

    /**
     * 来源类型：file/url/manual/api
     */
    private String sourceType;

    /**
     * 来源地址，如文件路径或网页URL
     */
    private String sourceUri;

    /**
     * 文件名
     */
    private String fileName;

    /**
     * 文件类型，如 pdf/txt/md/docx/html
     */
    private String fileType;

    /**
     * 文件大小，单位字节
     */
    private Long fileSize;

    /**
     * 内容哈希，用于去重
     */
    private String contentHash;

    /**
     * 语言
     */
    private String language;

    /**
     * 处理状态：pending/processing/completed/failed
     */
    private String status;

    /**
     * 扩展信息
     */
    @Builder.Default
    private Map<String, Object> metadata = new HashMap<>();

    public KnowledgeDocumentEntity toEntity() {
        return KnowledgeDocumentEntity.builder()
                .id(id)
                .kbId(kbId)
                .title(title)
                .sourceType(sourceType)
                .sourceUri(sourceUri)
                .fileName(fileName)
                .fileType(fileType)
                .fileSize(fileSize)
                .contentHash(contentHash)
                .language(language)
                .status(status)
                .metadata(metadata)
                .build();
    }
}
