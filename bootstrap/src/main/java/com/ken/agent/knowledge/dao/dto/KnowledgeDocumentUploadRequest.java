package com.ken.agent.knowledge.dao.dto;

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
public class KnowledgeDocumentUploadRequest {

    /**
     * 文档标题，不传时可使用文件名。
     */
    private String title;

    /**
     * 来源类型：file/url/manual/api
     */
    @Builder.Default
    private String sourceType = "file";

    /**
     * 来源地址，如原始文件路径或网页URL
     */
    private String sourceUri;

    /**
     * 语言
     */
    @Builder.Default
    private String language = "zh";

    /**
     * 分块策略：structure_aware/fixed_size
     */
    @Builder.Default
    private String chunkStrategy = "structure_aware";

    /**
     * 分块参数配置
     */
    @Builder.Default
    private Map<String, Object> chunkConfig = new HashMap<>();

    /**
     * 文档扩展信息
     */
    @Builder.Default
    private Map<String, Object> metadata = new HashMap<>();
}
