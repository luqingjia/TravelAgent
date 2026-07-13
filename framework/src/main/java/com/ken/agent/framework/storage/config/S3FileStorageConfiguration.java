package com.ken.agent.framework.storage.config;

import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.util.StringUtils;
import software.amazon.awssdk.auth.credentials.AwsBasicCredentials;
import software.amazon.awssdk.auth.credentials.StaticCredentialsProvider;
import software.amazon.awssdk.regions.Region;
import software.amazon.awssdk.services.s3.S3Client;
import software.amazon.awssdk.services.s3.S3Configuration;
import software.amazon.awssdk.services.s3.presigner.S3Presigner;

import java.net.URI;

/**
 * S3/RustFS 客户端配置。
 */
@Configuration
@EnableConfigurationProperties(S3StorageProperties.class)
@ConditionalOnProperty(prefix = "agent.storage.s3", name = "enabled", havingValue = "true")
public class S3FileStorageConfiguration {

    /**
     * 创建 S3 同步客户端。
     *
     * @param properties S3 存储配置
     * @return S3 同步客户端
     */
    @Bean
    public S3Client s3Client(S3StorageProperties properties) {
        var builder = S3Client.builder()
                .region(Region.of(properties.getRegion()))
                .credentialsProvider(credentialsProvider(properties))
                .serviceConfiguration(s3Configuration(properties));
        if (StringUtils.hasText(properties.getEndpoint())) {
            builder.endpointOverride(URI.create(properties.getEndpoint()));
        }
        return builder.build();
    }

    /**
     * 创建 S3 预签名客户端。
     *
     * @param properties S3 存储配置
     * @return S3 预签名客户端
     */
    @Bean
    public S3Presigner s3Presigner(S3StorageProperties properties) {
        var builder = S3Presigner.builder()
                .region(Region.of(properties.getRegion()))
                .credentialsProvider(credentialsProvider(properties))
                .serviceConfiguration(s3Configuration(properties));
        if (StringUtils.hasText(properties.getEndpoint())) {
            builder.endpointOverride(URI.create(properties.getEndpoint()));
        }
        return builder.build();
    }

    /**
     * 创建静态凭证提供器。
     *
     * @param properties S3 存储配置
     * @return 静态凭证提供器
     */
    private StaticCredentialsProvider credentialsProvider(S3StorageProperties properties) {
        return StaticCredentialsProvider.create(
                AwsBasicCredentials.create(properties.getAccessKey(), properties.getSecretKey()));
    }

    /**
     * 创建 S3 服务配置。
     *
     * @param properties S3 存储配置
     * @return S3 服务配置
     */
    private S3Configuration s3Configuration(S3StorageProperties properties) {
        return S3Configuration.builder()
                .pathStyleAccessEnabled(properties.isPathStyleAccess())
                .build();
    }
}
