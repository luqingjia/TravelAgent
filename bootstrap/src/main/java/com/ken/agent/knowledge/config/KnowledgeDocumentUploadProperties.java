package com.ken.agent.knowledge.config;

import lombok.Data;
import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.stereotype.Component;
import org.springframework.util.unit.DataSize;

import java.util.LinkedHashSet;
import java.util.Set;

/**
 * 文档上传业务限制。
 *
 * <p>配置前缀为 {@code agent.knowledge.document.upload}。默认允许常见文档格式且单文件不超过
 * 50MB；部署时可以通过 application.yml 对应配置或环境变量覆盖，修改后重启生效。</p>
 */
@Data
@Component
@ConfigurationProperties(prefix = "agent.knowledge.document.upload")
public class KnowledgeDocumentUploadProperties {

    /** 允许上传的扩展名，不需要写开头的点。 */
    private Set<String> allowedExtensions = new LinkedHashSet<>(Set.of(
            "pdf", "doc", "docx", "txt", "md", "markdown", "html", "htm"));

    /** 业务允许的单个文件最大大小。 */
    private DataSize maxSize = DataSize.ofMegabytes(50);
}
