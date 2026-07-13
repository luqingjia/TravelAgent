package com.ken.agent.framework.storage.config;

import lombok.Data;
import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * S3 兼容对象存储配置。
 *
 * <p>RustFS 兼容 S3 协议，因此也使用这组配置。</p>
 */
@Data
@ConfigurationProperties(prefix = "agent.storage.s3")
public class S3StorageProperties {

    /**
     * 是否启用 S3/RustFS 文件存储。
     */
    private boolean enabled;

    /**
     * 默认存储桶名称。
     */
    private String bucketName;

    /**
     * S3 兼容服务地址，例如 http://localhost:9000。
     */
    private String endpoint;

    /**
     * S3 区域。RustFS 可使用 us-east-1。
     */
    private String region = "us-east-1";

    /**
     * 访问密钥。
     */
    private String accessKey;

    /**
     * 访问密钥对应的 Secret。
     */
    private String secretKey;

    /**
     * 是否启用 path-style 访问。RustFS 通常需要开启。
     */
    private boolean pathStyleAccess = true;
}
