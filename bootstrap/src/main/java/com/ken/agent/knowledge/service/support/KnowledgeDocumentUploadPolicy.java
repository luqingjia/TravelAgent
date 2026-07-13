package com.ken.agent.knowledge.service.support;

import com.ken.agent.framework.exception.ClientException;
import com.ken.agent.knowledge.config.KnowledgeDocumentUploadProperties;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Component;
import org.springframework.util.StringUtils;
import org.springframework.web.multipart.MultipartFile;

import java.util.Locale;

/**
 * 上传文件进入对象存储前的业务校验器。
 *
 * <p>这里只判断文件是否为空、扩展名是否允许、声明大小是否超限，不读取文件内容，
 * 因而超大文件和错误格式可以尽早被拒绝。</p>
 */
@Component
@RequiredArgsConstructor
public class KnowledgeDocumentUploadPolicy {

    private final KnowledgeDocumentUploadProperties properties;

    /**
     * 按当前配置校验上传文件。
     *
     * @param file 待上传文件
     * @throws ClientException 文件为空、扩展名不允许或大小超限时抛出
     */
    public void validate(MultipartFile file) {
        if (file == null || file.isEmpty()) {
            throw new ClientException("上传文件不能为空");
        }

        String extension = StringUtils.getFilenameExtension(file.getOriginalFilename());
        String normalizedExtension = extension == null ? "" : extension.toLowerCase(Locale.ROOT);
        boolean allowed = StringUtils.hasText(normalizedExtension)
                && properties.getAllowedExtensions().stream()
                .map(value -> value == null ? "" : value.replaceFirst("^\\.", "").toLowerCase(Locale.ROOT))
                .anyMatch(normalizedExtension::equals);
        if (!allowed) {
            String displayExtension = StringUtils.hasText(normalizedExtension) ? normalizedExtension : "无扩展名";
            throw new ClientException("不支持的文件类型: " + displayExtension);
        }

        long maxBytes = properties.getMaxSize().toBytes();
        if (file.getSize() > maxBytes) {
            throw new ClientException("文件大小超过限制: " + properties.getMaxSize());
        }
    }
}
