package com.ken.agent.knowledge.dao.vo;

import com.ken.agent.knowledge.dao.entity.KnowledgeDocumentEntity;
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
public class KnowledgeDocumentVO {

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
     * 分块数量
     */
    private Integer chunkCount;

    /**
     * 分块策略
     */
    private String chunkStrategy;

    /**
     * 分块参数配置
     */
    private Map<String, Object> chunkConfig;

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

    public static KnowledgeDocumentVO fromEntity(KnowledgeDocumentEntity entity) {
        if (entity == null) {
            return null;
        }
        return KnowledgeDocumentVO.builder()
                .id(entity.getId())
                .kbId(entity.getKbId())
                .title(entity.getTitle())
                .sourceType(entity.getSourceType())
                .sourceUri(entity.getSourceUri())
                .fileName(entity.getFileName())
                .fileType(entity.getFileType())
                .fileSize(entity.getFileSize())
                .contentHash(entity.getContentHash())
                .language(entity.getLanguage())
                .status(entity.getStatus())
                .chunkCount(entity.getChunkCount())
                .chunkStrategy(entity.getChunkStrategy())
                .chunkConfig(entity.getChunkConfig())
                .metadata(entity.getMetadata())
                .createTime(entity.getCreateTime())
                .updateTime(entity.getUpdateTime())
                .build();
    }
}
