package com.ken.agent.framework.storage.dto;

import lombok.AllArgsConstructor;
import lombok.Builder;
import lombok.Data;
import lombok.NoArgsConstructor;

/**
 * 已存储文件信息。
 */
@Data
@Builder
@NoArgsConstructor
@AllArgsConstructor
public class StoredFileDTO {

    /**
     * 文件存储地址，例如 s3://bucket/key。
     */
    private String url;

    /**
     * 识别后的文件类型，例如 pdf、txt、docx。
     */
    private String detectedType;

    /**
     * 文件大小，单位字节。
     */
    private Long size;

    /**
     * 上传时的原始文件名。
     */
    private String originalFilename;
}
