package com.ken.agent.framework.storage.support;

import org.springframework.util.StringUtils;

/**
 * 文件类型识别工具。
 */
public final class FileTypeDetector {

    private FileTypeDetector() {
    }

    /**
     * 根据文件名和内容类型识别业务文件类型。
     *
     * @param originalFilename 原始文件名
     * @param contentType      内容类型
     * @return 文件类型，无法识别时返回 unknown
     */
    public static String detectType(String originalFilename, String contentType) {
        String extension = StringUtils.getFilenameExtension(originalFilename);
        if (StringUtils.hasText(extension)) {
            return extension.toLowerCase();
        }
        if (!StringUtils.hasText(contentType)) {
            return "unknown";
        }
        int slashIndex = contentType.lastIndexOf('/');
        if (slashIndex < 0 || slashIndex == contentType.length() - 1) {
            return contentType.toLowerCase();
        }
        return contentType.substring(slashIndex + 1).toLowerCase();
    }
}
