package com.ken.agent.framework.storage;

import com.ken.agent.framework.storage.dto.StoredFileDTO;

import java.io.InputStream;

/**
 * 文件存储服务接口。
 *
 * <p>业务代码只依赖该接口，不直接依赖本地文件系统、RustFS 或其他对象存储实现。</p>
 */
public interface FileStorageService {

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
    StoredFileDTO upload(String bucketName, InputStream content, long size, String originalFilename, String contentType);

    /**
     * 上传字节数组内容。
     *
     * @param bucketName       存储桶名称
     * @param content          文件内容
     * @param originalFilename 原始文件名
     * @param contentType      内容类型，可为空
     * @return 已存储文件信息
     */
    StoredFileDTO upload(String bucketName, byte[] content, String originalFilename, String contentType);

    /**
     * 打开已存储文件的读取流。
     *
     * @param url 文件存储地址
     * @return 文件内容输入流，调用方负责关闭
     */
    InputStream openStream(String url);

    /**
     * 根据存储地址删除文件。
     *
     * @param url 文件存储地址
     */
    void deleteByUrl(String url);

    /**
     * 使用 SDK 原生上传方式上传输入流。
     *
     * <p>该方法优先保证兼容性，适用于文件较小或需要绕过预签名 URL 上传限制的场景。</p>
     *
     * @param bucketName       存储桶名称
     * @param content          文件内容输入流
     * @param size             文件大小，单位字节
     * @param originalFilename 原始文件名
     * @param contentType      内容类型，可为空
     * @return 已存储文件信息
     */
    StoredFileDTO reliableUpload(String bucketName, InputStream content, long size, String originalFilename, String contentType);
}
