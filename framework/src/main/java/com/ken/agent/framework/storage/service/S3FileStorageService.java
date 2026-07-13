package com.ken.agent.framework.storage.service;

import cn.hutool.core.util.IdUtil;
import com.ken.agent.framework.errorcode.BaseErrorCode;
import com.ken.agent.framework.exception.ServiceException;
import com.ken.agent.framework.storage.FileStorageService;
import com.ken.agent.framework.storage.dto.StoredFileDTO;
import com.ken.agent.framework.storage.support.FileTypeDetector;
import lombok.RequiredArgsConstructor;
import org.apache.tika.Tika;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.stereotype.Service;
import org.springframework.util.Assert;
import org.springframework.util.StringUtils;
import software.amazon.awssdk.core.sync.RequestBody;
import software.amazon.awssdk.services.s3.S3Client;
import software.amazon.awssdk.services.s3.model.PutObjectRequest;
import software.amazon.awssdk.services.s3.presigner.S3Presigner;
import software.amazon.awssdk.services.s3.presigner.model.PresignedPutObjectRequest;

import java.io.ByteArrayInputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.time.LocalDate;

/**
 * 基于 S3 协议的文件存储服务。
 *
 * <p>RustFS 兼容 S3 协议，因此可以复用该实现。默认上传路径使用日期目录和雪花 ID，
 * 返回地址格式为 {@code s3://bucket/key}。</p>
 */
@Service
@RequiredArgsConstructor
@ConditionalOnProperty(prefix = "agent.storage.s3", name = "enabled", havingValue = "true")
public class S3FileStorageService implements FileStorageService {

    private static final Tika TIKA = new Tika();
    private static final Duration PRESIGN_DURATION = Duration.ofMinutes(10);
    private static final int CONNECT_TIMEOUT_MS = 10_000;
    private static final int READ_TIMEOUT_MS = 60_000;

    private final S3Client s3Client;
    private final S3Presigner s3Presigner;

    /**
     * 上传输入流内容。
     *
     * @param bucketName       存储桶名称
     * @param content          文件内容输入流
     * @param size             文件大小，单位字节
     * @param originalFilename 原始文件名
     * @param contentType      内容类型，可为空
     * @return 已存储文件信息
     */
    @Override
    public StoredFileDTO upload(String bucketName, InputStream content, long size,
                                String originalFilename, String contentType) {
        validateBucketName(bucketName);
        Assert.notNull(content, "上传内容不能为空");
        Assert.isTrue(size >= 0, "上传内容大小不能小于 0");
        String detected = resolveContentType(originalFilename, contentType);
        return streamUploadToS3(bucketName, content, size, originalFilename, detected);
    }

    /**
     * 上传字节数组内容。
     *
     * @param bucketName       存储桶名称
     * @param content          文件内容
     * @param originalFilename 原始文件名
     * @param contentType      内容类型，可为空
     * @return 已存储文件信息
     */
    @Override
    public StoredFileDTO upload(String bucketName, byte[] content, String originalFilename, String contentType) {
        validateBucketName(bucketName);
        Assert.notNull(content, "上传内容不能为空");
        String detected = resolveContentType(originalFilename, contentType);
        return streamUploadToS3(bucketName, new ByteArrayInputStream(content), content.length, originalFilename, detected);
    }

    /**
     * 打开 S3 地址对应对象的读取流。
     *
     * @param url 文件存储地址，格式为 s3://bucket/key
     * @return 文件内容输入流，调用方负责关闭
     */
    @Override
    public InputStream openStream(String url) {
        S3Location location = parseS3Url(url);
        return s3Client.getObject(builder -> builder.bucket(location.bucket()).key(location.key()));
    }

    /**
     * 根据 S3 地址删除对象。
     *
     * @param url 文件存储地址，格式为 s3://bucket/key
     */
    @Override
    public void deleteByUrl(String url) {
        S3Location location = parseS3Url(url);
        s3Client.deleteObject(builder -> builder.bucket(location.bucket()).key(location.key()));
    }

    /**
     * 使用 SDK 原生 putObject 上传。
     *
     * <p>该方法优先保证兼容性，适用于小文件或预签名 URL 上传不可用的场景。</p>
     *
     * @param bucketName       存储桶名称
     * @param content          文件内容输入流
     * @param size             文件大小，单位字节
     * @param originalFilename 原始文件名
     * @param contentType      内容类型，可为空
     * @return 已存储文件信息
     */
    @Override
    public StoredFileDTO reliableUpload(String bucketName, InputStream content, long size,
                                        String originalFilename, String contentType) {
        validateBucketName(bucketName);
        Assert.notNull(content, "上传内容不能为空");
        Assert.isTrue(size >= 0, "上传内容大小不能小于 0");
        String detected = resolveContentType(originalFilename, contentType);
        String s3Key = generateS3Key(originalFilename);

        s3Client.putObject(PutObjectRequest.builder()
                        .bucket(bucketName)
                        .key(s3Key)
                        .contentType(detected)
                        .build(),
                RequestBody.fromInputStream(content, size));

        return buildStoredFileDTO(toS3Url(bucketName, s3Key), originalFilename, detected, size);
    }

    /**
     * 通过预签名 URL 流式上传到 S3。
     *
     * @param bucketName          存储桶名称
     * @param inputStream         文件内容输入流
     * @param size                文件大小，单位字节
     * @param originalFilename    原始文件名
     * @param detectedContentType 已识别的内容类型
     * @return 已存储文件信息
     */
    private StoredFileDTO streamUploadToS3(String bucketName, InputStream inputStream,
                                           long size, String originalFilename,
                                           String detectedContentType) {
        String s3Key = generateS3Key(originalFilename);
        PresignedPutObjectRequest presignedRequest = s3Presigner.presignPutObject(builder -> builder
                .signatureDuration(PRESIGN_DURATION)
                .putObjectRequest(PutObjectRequest.builder()
                        .bucket(bucketName)
                        .key(s3Key)
                        .contentType(detectedContentType)
                        .build()));

        try {
            streamPutViaPresignedUrl(presignedRequest, inputStream, size, detectedContentType);
        } catch (IOException ex) {
            throw new ServiceException("S3 流式上传失败", ex, BaseErrorCode.SERVICE_ERROR);
        }

        return buildStoredFileDTO(toS3Url(bucketName, s3Key), originalFilename, detectedContentType, size);
    }

    /**
     * 使用预签名 URL 执行 HTTP PUT 流式上传。
     *
     * @param presignedRequest 预签名 PUT 请求
     * @param inputStream      文件内容输入流
     * @param size             文件大小，单位字节
     * @param contentType      内容类型，可为空
     * @throws IOException 网络或对象存储返回非 2xx 状态时抛出
     */
    private void streamPutViaPresignedUrl(PresignedPutObjectRequest presignedRequest,
                                          InputStream inputStream,
                                          long size,
                                          String contentType) throws IOException {
        HttpURLConnection connection = (HttpURLConnection) presignedRequest.url().openConnection();
        try {
            connection.setDoOutput(true);
            connection.setRequestMethod("PUT");
            connection.setFixedLengthStreamingMode(size);
            connection.setConnectTimeout(CONNECT_TIMEOUT_MS);
            connection.setReadTimeout(READ_TIMEOUT_MS);

            presignedRequest.signedHeaders()
                    .forEach((key, values) -> values.forEach(value -> connection.addRequestProperty(key, value)));
            if (StringUtils.hasText(contentType)) {
                connection.setRequestProperty("Content-Type", contentType);
            }

            try (OutputStream outputStream = connection.getOutputStream()) {
                inputStream.transferTo(outputStream);
            }

            int responseCode = connection.getResponseCode();
            if (responseCode < 200 || responseCode >= 300) {
                throw new IOException("HTTP " + responseCode + ", body=" + readErrorStream(connection));
            }
        } finally {
            connection.disconnect();
        }
    }

    /**
     * 读取对象存储错误响应体。
     *
     * @param connection HTTP 连接
     * @return 错误响应体文本
     */
    private String readErrorStream(HttpURLConnection connection) {
        try (InputStream errorStream = connection.getErrorStream()) {
            return errorStream != null
                    ? new String(errorStream.readAllBytes(), StandardCharsets.UTF_8)
                    : "(empty)";
        } catch (IOException ex) {
            return "(read error: " + ex.getMessage() + ")";
        }
    }

    /**
     * 构造 S3 地址。
     *
     * @param bucket 存储桶名称
     * @param key    对象键
     * @return S3 地址
     */
    private String toS3Url(String bucket, String key) {
        return "s3://" + bucket + "/" + key;
    }

    /**
     * 解析 S3 地址。
     *
     * @param url S3 地址，格式为 s3://bucket/key
     * @return 解析后的 S3 位置
     */
    private S3Location parseS3Url(String url) {
        URI uri = URI.create(url);
        if (!"s3".equalsIgnoreCase(uri.getScheme())) {
            throw new IllegalArgumentException("Unsupported url scheme: " + url);
        }
        String bucket = uri.getHost();
        String path = uri.getPath();
        if (!StringUtils.hasText(bucket)) {
            throw new IllegalArgumentException("Invalid s3 url, bucket missing: " + url);
        }
        String key = path != null && path.startsWith("/") ? path.substring(1) : path;
        if (!StringUtils.hasText(key)) {
            throw new IllegalArgumentException("Invalid s3 url, key missing: " + url);
        }
        return new S3Location(bucket, key);
    }

    /**
     * 提取文件后缀。
     *
     * @param filename 文件名
     * @return 文件后缀，不包含点号
     */
    private String extractSuffix(String filename) {
        String extension = StringUtils.getFilenameExtension(filename);
        return StringUtils.hasText(extension) ? extension.trim() : "";
    }

    /**
     * 生成对象存储键。
     *
     * @param originalFilename 原始文件名
     * @return 对象存储键
     */
    private String generateS3Key(String originalFilename) {
        String suffix = extractSuffix(originalFilename);
        String datePath = LocalDate.now().toString().replace("-", "/");
        String key = datePath + "/" + IdUtil.getSnowflakeNextIdStr();
        return suffix.isBlank() ? key : key + "." + suffix;
    }

    /**
     * 校验存储桶名称。
     *
     * @param bucketName 存储桶名称
     */
    private void validateBucketName(String bucketName) {
        Assert.hasText(bucketName, "bucketName 不能为空");
    }

    /**
     * 构建已存储文件信息。
     *
     * @param url              文件存储地址
     * @param originalFilename 原始文件名
     * @param contentType      内容类型
     * @param size             文件大小，单位字节
     * @return 已存储文件信息
     */
    private StoredFileDTO buildStoredFileDTO(String url, String originalFilename,
                                             String contentType, long size) {
        return StoredFileDTO.builder()
                .url(url)
                .detectedType(FileTypeDetector.detectType(originalFilename, contentType))
                .size(size)
                .originalFilename(originalFilename)
                .build();
    }

    /**
     * 解析内容类型。
     *
     * @param originalFilename 原始文件名
     * @param contentType      外部传入的内容类型
     * @return 内容类型，无法识别时返回 null
     */
    private String resolveContentType(String originalFilename, String contentType) {
        if (StringUtils.hasText(contentType)) {
            return contentType;
        }
        if (StringUtils.hasText(originalFilename)) {
            return TIKA.detect(originalFilename);
        }
        return null;
    }

    /**
     * S3 对象位置。
     *
     * @param bucket 存储桶名称
     * @param key    对象键
     */
    private record S3Location(String bucket, String key) {
    }
}
