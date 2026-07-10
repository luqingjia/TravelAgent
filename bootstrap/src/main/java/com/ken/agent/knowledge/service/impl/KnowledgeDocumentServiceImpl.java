package com.ken.agent.knowledge.service.impl;

import cn.hutool.core.util.IdUtil;
import com.baomidou.mybatisplus.core.conditions.query.LambdaQueryWrapper;
import com.baomidou.mybatisplus.core.metadata.IPage;
import com.baomidou.mybatisplus.core.toolkit.Wrappers;
import com.baomidou.mybatisplus.extension.plugins.pagination.Page;
import com.baomidou.mybatisplus.extension.service.impl.ServiceImpl;
import com.ken.agent.core.chunk.ChunkEmbeddingService;
import com.ken.agent.core.chunk.ChunkingEnum;
import com.ken.agent.core.chunk.ChunkingOptions;
import com.ken.agent.core.chunk.ChunkingStrategy;
import com.ken.agent.core.chunk.ChunkingStrategyFactory;
import com.ken.agent.core.chunk.VectorChunk;
import com.ken.agent.core.parser.DocumentParser;
import com.ken.agent.core.parser.DocumentParserSelector;
import com.ken.agent.framework.errorcode.BaseErrorCode;
import com.ken.agent.framework.exception.ClientException;
import com.ken.agent.framework.exception.ServiceException;
import com.ken.agent.framework.storage.FileStorageService;
import com.ken.agent.framework.storage.config.S3StorageProperties;
import com.ken.agent.framework.storage.dto.StoredFileDTO;
import com.ken.agent.knowledge.controller.request.KnowledgeDocumentChunkRequest;
import com.ken.agent.knowledge.controller.request.KnowledgeDocumentPageRequest;
import com.ken.agent.knowledge.dao.dto.KnowledgeDocumentUploadRequest;
import com.ken.agent.knowledge.dao.entity.KnowledgeBaseEntity;
import com.ken.agent.knowledge.dao.entity.KnowledgeChunkEntity;
import com.ken.agent.knowledge.dao.entity.KnowledgeDocumentEntity;
import com.ken.agent.knowledge.dao.mapper.KnowledgeBaseMapper;
import com.ken.agent.knowledge.dao.mapper.KnowledgeDocumentMapper;
import com.ken.agent.knowledge.dao.vo.KnowledgeDocumentVO;
import com.ken.agent.knowledge.enums.DocumentStatus;
import com.ken.agent.knowledge.enums.SourceType;
import com.ken.agent.knowledge.service.KnowledgeChunkService;
import com.ken.agent.knowledge.service.KnowledgeDocumentService;
import com.ken.agent.knowledge.service.KnowledgeVectorService;
import com.ken.agent.knowledge.service.support.KnowledgeDocumentUploadPolicy;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.ObjectProvider;
import org.springframework.dao.DuplicateKeyException;
import org.springframework.stereotype.Service;
import org.springframework.transaction.support.TransactionOperations;
import org.springframework.util.StringUtils;
import org.springframework.web.multipart.MultipartFile;

import java.io.IOException;
import java.io.InputStream;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.HashMap;
import java.util.HexFormat;
import java.util.List;
import java.util.Map;

/**
 * 文档上传和切分流程的具体实现。
 *
 * <p>这个类把一篇文档从“用户刚上传”带到“可以被向量检索”的状态。为了让失败重试简单、
 * 状态也看得见，流程故意拆成两个接口：上传只负责保存原文件和文档记录；切分接口再负责读取文件、
 * 解析文字、分块、生成向量并替换结果。</p>
 *
 * <p>最重要的保护规则是：耗时的文件解析和模型调用不占用数据库事务，而且不会提前删除旧结果。
 * 只有所有新分块和向量都准备好以后，才开启一个短事务整体替换。重试失败时，旧的可检索结果仍然在。</p>
 */
@Slf4j
@Service
@RequiredArgsConstructor
public class KnowledgeDocumentServiceImpl extends ServiceImpl<KnowledgeDocumentMapper, KnowledgeDocumentEntity>
        implements KnowledgeDocumentService {

    private static final String DEFAULT_BUCKET = "knowledge";
    private static final String DEFAULT_LANGUAGE = "zh";
    private static final String DEFAULT_CHUNK_STRATEGY = "structure_aware";
    private static final String DUPLICATE_DOCUMENT_MESSAGE = "同一知识库已存在重复内容的文档";

    private final KnowledgeBaseMapper knowledgeBaseMapper;
    private final KnowledgeChunkService knowledgeChunkService;
    private final KnowledgeVectorService knowledgeVectorService;
    private final DocumentParserSelector parserSelector;
    private final ChunkingStrategyFactory chunkingStrategyFactory;
    private final ChunkEmbeddingService chunkEmbeddingService;
    private final ObjectProvider<FileStorageService> fileStorageServiceProvider;
    private final ObjectProvider<S3StorageProperties> s3StoragePropertiesProvider;
    private final TransactionOperations transactionOperations;
    private final KnowledgeDocumentUploadPolicy uploadPolicy;

    /**
     * {@inheritDoc}
     *
     * <p>处理顺序是“校验文件 → 读取并算内容指纹 → 查重 → 上传对象存储 → 保存数据库记录”。
     * 这个顺序既避免无效文件占用存储空间，也让数据库保存失败时只需要删除刚上传的一个文件。</p>
     */
    @Override
    public KnowledgeDocumentVO upload(String kbId, KnowledgeDocumentUploadRequest requestParam, MultipartFile file) {
        requireKnowledgeBase(kbId);
        uploadPolicy.validate(file);

        KnowledgeDocumentUploadRequest request = requestParam == null
                ? KnowledgeDocumentUploadRequest.builder().build()
                : requestParam;
        SourceType sourceType = SourceType.normalize(request.getSourceType());
        if (sourceType != SourceType.FILE) {
            throw new ClientException("当前仅支持文件上传类型");
        }

        // 去重看文件内容，不看文件名：同名但内容不同可以上传，改名后的同一内容仍能识别出来。
        byte[] content = readFileContent(file);
        String fileName = resolveFileName(file);
        String contentHash = sha256Hex(content);
        if (getBaseMapper().existsActiveByKbIdAndContentHash(kbId, contentHash)) {
            throw new ClientException(DUPLICATE_DOCUMENT_MESSAGE);
        }
        StoredFileDTO storedFile = uploadToRustFs(content, fileName, file.getContentType());

        KnowledgeDocumentEntity entity = KnowledgeDocumentEntity.builder()
                .id(IdUtil.getSnowflakeNextIdStr())
                .kbId(kbId)
                .title(defaultIfBlank(request.getTitle(), fileName))
                .sourceType(sourceType.getValue())
                .sourceUri(storedFile.getUrl())
                .fileName(fileName)
                .fileType(defaultIfBlank(storedFile.getDetectedType(), StringUtils.getFilenameExtension(fileName)))
                .fileSize(storedFile.getSize())
                .contentHash(contentHash)
                .language(defaultIfBlank(request.getLanguage(), DEFAULT_LANGUAGE))
                .status(DocumentStatus.PENDING.getCode())
                .chunkCount(0)
                .chunkStrategy(defaultIfBlank(request.getChunkStrategy(), DEFAULT_CHUNK_STRATEGY))
                .chunkConfig(request.getChunkConfig())
                .metadata(resolveMetadata(request.getMetadata(), storedFile))
                .build();
        // 文件和数据库不是同一个事务资源。数据库没保存成功时，用补偿删除避免留下无主文件。
        try {
            if (!save(entity)) {
                throw new ServiceException("创建文档记录失败");
            }
        } catch (RuntimeException ex) {
            deleteStoredFileQuietly(storedFile.getUrl());
            if (ex instanceof DuplicateKeyException) {
                throw new ClientException(DUPLICATE_DOCUMENT_MESSAGE);
            }
            throw ex;
        }
        return KnowledgeDocumentVO.fromEntity(entity);
    }

    /**
     * {@inheritDoc}
     *
     * <p>先用一条带条件的 UPDATE 抢到处理权，防止两个请求同时切同一篇文档。随后在事务外准备
     * 全部新结果，准备成功后才调用短事务替换；失败则把状态改为 failed，并保存最近一次错误。</p>
     */
    @Override
    public KnowledgeDocumentVO startChunk(String docId, KnowledgeDocumentChunkRequest requestParam) {
        KnowledgeDocumentEntity document = requireDocument(docId);
        if (DocumentStatus.PROCESSING.getCode().equals(document.getStatus())) {
            throw new ClientException("文档正在切分处理中，请稍后再试");
        }

        KnowledgeDocumentChunkRequest request = requestParam == null ? new KnowledgeDocumentChunkRequest() : requestParam;
        ChunkingEnum chunkingEnum = resolveChunkingEnum(defaultIfBlank(request.getChunkStrategy(), document.getChunkStrategy()));
        Map<String, Object> chunkConfig = request.getChunkConfig() == null || request.getChunkConfig().isEmpty()
                ? defaultMap(document.getChunkConfig())
                : request.getChunkConfig();

        // 前面的状态判断方便快速给出提示；这里的条件更新才是并发下真正可靠的“抢锁”。
        if (getBaseMapper().tryMarkProcessing(docId) == 0) {
            throw new ClientException("文档正在切分处理中，请稍后再试");
        }
        long startTime = System.currentTimeMillis();
        try {
            // 解析、分块和模型调用都可能较慢，先在事务外做完，避免长时间占住数据库连接和锁。
            ChunkProcessResult processResult = extractChunkAndEmbed(document, chunkingEnum, chunkConfig);
            persistChunksAndVectors(document, chunkingEnum, chunkConfig, processResult.chunks());
            log.info("文档切分完成, docId={}, chunks={}, duration={}ms",
                    docId, processResult.chunks().size(), System.currentTimeMillis() - startTime);
            return getDocument(docId);
        } catch (Exception ex) {
            log.error("文档切分失败, docId={}", docId, ex);
            String errorMessage = defaultIfBlank(ex.getMessage(), ex.getClass().getSimpleName());
            markStatus(document, DocumentStatus.FAILED, errorMessage);
            throw ex instanceof ClientException ? (ClientException) ex
                    : new ServiceException("文档切分失败: " + ex.getMessage(), ex, BaseErrorCode.SERVICE_ERROR);
        }
    }

    /**
     * {@inheritDoc}
     *
     * <p>先在一个数据库事务中删除分块、删除向量、删除文档记录；事务成功后再删对象存储文件。
     * 对象存储删除失败只记录日志，因为此时数据库中已经没有入口，不能为了清理文件把已提交的
     * 数据库删除假装成失败。</p>
     */
    @Override
    public void deleteDocument(String docId) {
        KnowledgeDocumentEntity document = requireDocument(docId);
        if (DocumentStatus.PROCESSING.getCode().equals(document.getStatus())) {
            throw new ClientException("文档正在切分处理中，无法删除");
        }
        transactionOperations.executeWithoutResult(status -> {
            knowledgeChunkService.deleteByDocumentId(docId);
            knowledgeVectorService.deleteDocumentVectors(docId);
            removeById(docId);
        });
        deleteStoredFileQuietly(document.getSourceUri());
    }

    /** {@inheritDoc} */
    @Override
    public KnowledgeDocumentVO getDocument(String docId) {
        return KnowledgeDocumentVO.fromEntity(requireDocument(docId));
    }

    /** {@inheritDoc} */
    @Override
    public IPage<KnowledgeDocumentVO> pageDocuments(String kbId, KnowledgeDocumentPageRequest requestParam) {
        requireKnowledgeBase(kbId);
        KnowledgeDocumentPageRequest request = requestParam == null ? new KnowledgeDocumentPageRequest() : requestParam;
        LambdaQueryWrapper<KnowledgeDocumentEntity> wrapper = Wrappers.lambdaQuery(KnowledgeDocumentEntity.class)
                .eq(KnowledgeDocumentEntity::getKbId, kbId)
                .like(StringUtils.hasText(request.getKeyword()), KnowledgeDocumentEntity::getTitle, request.getKeyword())
                .eq(StringUtils.hasText(request.getStatus()), KnowledgeDocumentEntity::getStatus, request.getStatus())
                .orderByDesc(KnowledgeDocumentEntity::getUpdateTime);
        return page(new Page<>(request.getCurrent(), request.getSize()), wrapper)
                .convert(KnowledgeDocumentVO::fromEntity);
    }

    /**
     * 从对象存储读取原文件，并准备一套完整、可落库的新分块结果。
     *
     * <p>依次完成文件解析、按策略切分、补充检索元数据、批量生成向量和最终完整性校验。
     * 这个方法不写业务表，所以中途失败不会碰到旧分块和旧向量。</p>
     *
     * @param document 待处理文档
     * @param chunkingEnum 本次切分策略
     * @param chunkConfig 本次切分参数
     * @return 已经包含 1536 维向量的新分块结果
     * @throws ServiceException 文件读取、解析、切分或向量结果不合格时抛出
     */
    private ChunkProcessResult extractChunkAndEmbed(KnowledgeDocumentEntity document,
                                                    ChunkingEnum chunkingEnum,
                                                    Map<String, Object> chunkConfig) {
        String text;
        FileStorageService storageService = requireFileStorageService();
        try (InputStream inputStream = storageService.openStream(document.getSourceUri())) {
            DocumentParser parser = parserSelector.selectByFile(document.getFileName(), document.getFileType());
            text = parser.extractText(inputStream, document.getFileName());
        } catch (IOException ex) {
            throw new ServiceException("关闭文件流失败", ex, BaseErrorCode.SERVICE_ERROR);
        }
        if (!StringUtils.hasText(text)) {
            throw new ServiceException("文档解析结果为空");
        }

        ChunkingOptions options = chunkingEnum.createOptions(chunkConfig);
        ChunkingStrategy strategy = chunkingStrategyFactory.getChunkingStrategy(chunkingEnum);
        List<VectorChunk> chunks = strategy.chunk(text, options);
        if (chunks == null || chunks.isEmpty()) {
            throw new ServiceException("文档未生成有效分块");
        }
        // 标题和文件名会随每个向量一起保存，检索返回结果时不必再额外查文档表才能展示来源。
        for (VectorChunk chunk : chunks) {
            Map<String, Object> metadata = new HashMap<>(chunk.getMetadata() == null ? Map.of() : chunk.getMetadata());
            metadata.put("title", document.getTitle());
            metadata.put("fileName", document.getFileName());
            chunk.setMetadata(metadata);
        }
        chunkEmbeddingService.embeddingText(chunks);
        validateProcessedChunks(chunks);
        return new ChunkProcessResult(chunks);
    }

    /**
     * 用已经准备好的新结果，原子替换文档的旧分块和旧向量。
     *
     * <p>事务中的顺序是：物理删除旧分块 → 删除旧向量 → 写入新分块 → 批量写入新向量 →
     * 标记文档完成。任一步抛错都会整体回滚，因此重试不会把旧结果删一半。</p>
     *
     * @param document 当前文档
     * @param chunkingEnum 实际采用的切分策略
     * @param chunkConfig 实际采用的切分参数
     * @param chunks 已经通过完整性校验的新分块
     */
    private void persistChunksAndVectors(KnowledgeDocumentEntity document,
                                         ChunkingEnum chunkingEnum,
                                         Map<String, Object> chunkConfig,
                                         List<VectorChunk> chunks) {
        List<KnowledgeChunkEntity> chunkEntities = chunks.stream()
                .map(chunk -> KnowledgeChunkEntity.builder()
                        .id(chunk.getChunkId())
                        .kbId(document.getKbId())
                        .documentId(document.getId())
                        .chunkIndex(chunk.getIndex())
                        .content(chunk.getContent())
                        .tokenCount(chunk.getContent() == null ? 0 : chunk.getContent().length())
                        .charCount(chunk.getContent() == null ? 0 : chunk.getContent().length())
                        .metadata(chunk.getMetadata())
                        .enabled((short) 1)
                        .build())
                .toList();

        // 事务只包数据库替换动作，不把文件解析和外部模型调用放进来，持续时间通常很短。
        transactionOperations.executeWithoutResult(status -> {
            knowledgeChunkService.deletePhysicallyByDocumentId(document.getId());
            knowledgeVectorService.deleteDocumentVectors(document.getId());
            if (!chunkEntities.isEmpty()) {
                knowledgeChunkService.saveBatch(chunkEntities);
                knowledgeVectorService.indexDocumentChunks(document.getKbId(), document.getId(), chunks);
            }
            KnowledgeDocumentEntity update = KnowledgeDocumentEntity.builder()
                    .id(document.getId())
                    .status(DocumentStatus.COMPLETED.getCode())
                    .chunkCount(chunkEntities.size())
                    .chunkStrategy(chunkingEnum.getValue())
                    .chunkConfig(chunkConfig)
                    .metadata(metadataWithoutLastError(document.getMetadata()))
                    .build();
            updateById(update);
        });
    }

    /**
     * 更新文档处理状态，并把本次失败原因放进现有元数据。
     *
     * <p>这里只维护“最近一次错误”，MVP 暂不保存每次执行的历史和耗时。</p>
     *
     * @param document 当前文档
     * @param status 要写入的状态
     * @param errorMessage 最近一次错误；为空时会清除旧错误
     */
    private void markStatus(KnowledgeDocumentEntity document, DocumentStatus status, String errorMessage) {
        KnowledgeDocumentEntity update = KnowledgeDocumentEntity.builder()
                .id(document.getId())
                .status(status.getCode())
                .metadata(mergeMetadata(document.getMetadata(), errorMessage))
                .build();
        updateById(update);
    }

    /**
     * 在替换旧数据之前，对所有新分块做最后一道完整性检查。
     *
     * @param chunks 新分块列表
     * @throws ServiceException 存在空文字、空向量或非 1536 维向量时抛出
     */
    private void validateProcessedChunks(List<VectorChunk> chunks) {
        for (VectorChunk chunk : chunks) {
            if (chunk == null || !StringUtils.hasText(chunk.getContent())) {
                throw new ServiceException("文档包含空分块");
            }
            if (chunk.getEmbedding() == null || chunk.getEmbedding().length == 0) {
                throw new ServiceException("文档分块向量不能为空");
            }
            if (chunk.getEmbedding().length != KnowledgeVectorService.EMBEDDING_DIMENSIONS) {
                throw new ServiceException(
                        "文档分块向量维度必须为 " + KnowledgeVectorService.EMBEDDING_DIMENSIONS);
            }
        }
    }

    /**
     * 复制元数据并移除上一次错误，供切分成功时保存。
     *
     * @param metadata 原有文档元数据
     * @return 不包含 {@code lastError} 的新 Map
     */
    private Map<String, Object> metadataWithoutLastError(Map<String, Object> metadata) {
        return mergeMetadata(metadata, null);
    }

    /**
     * 在不破坏文件存储信息等原有内容的前提下，写入或清除最近一次错误。
     *
     * @param metadata 原有元数据
     * @param errorMessage 新错误；为空表示清除错误
     * @return 可以安全修改并保存的新 Map
     */
    private Map<String, Object> mergeMetadata(Map<String, Object> metadata, String errorMessage) {
        Map<String, Object> resolved = metadata == null ? new HashMap<>() : new HashMap<>(metadata);
        if (StringUtils.hasText(errorMessage)) {
            resolved.put("lastError", errorMessage);
        } else {
            resolved.remove("lastError");
        }
        return resolved;
    }

    /**
     * 把已读入内存的文件上传到配置的对象存储桶。
     *
     * @param content 文件字节
     * @param fileName 文件名
     * @param contentType HTTP 内容类型
     * @return 对象存储返回的地址、大小和识别类型
     */
    private StoredFileDTO uploadToRustFs(byte[] content, String fileName, String contentType) {
        FileStorageService storageService = requireFileStorageService();
        String bucketName = s3StoragePropertiesProvider.stream()
                .map(S3StorageProperties::getBucketName)
                .filter(StringUtils::hasText)
                .findFirst()
                .orElse(DEFAULT_BUCKET);
        return storageService.reliableUpload(bucketName, new java.io.ByteArrayInputStream(content), content.length, fileName, contentType);
    }

    /**
     * 取得当前启用的文件存储服务。
     *
     * @return 文件存储服务
     * @throws ServiceException 项目未启用 S3/RustFS 存储时抛出
     */
    private FileStorageService requireFileStorageService() {
        FileStorageService storageService = fileStorageServiceProvider.getIfAvailable();
        if (storageService == null) {
            throw new ServiceException("RustFS 文件存储未启用，请配置 agent.storage.s3.enabled=true");
        }
        return storageService;
    }

    /**
     * 校验并读取上传目标知识库。
     *
     * @param kbId 知识库 ID
     * @return 已存在的知识库
     * @throws ClientException ID 为空或知识库不存在时抛出
     */
    private KnowledgeBaseEntity requireKnowledgeBase(String kbId) {
        if (!StringUtils.hasText(kbId)) {
            throw new ClientException("知识库ID不能为空");
        }
        KnowledgeBaseEntity knowledgeBase = knowledgeBaseMapper.selectById(kbId);
        if (knowledgeBase == null) {
            throw new ClientException("知识库不存在");
        }
        return knowledgeBase;
    }

    /**
     * 校验并读取文档。
     *
     * @param docId 文档 ID
     * @return 已存在的文档
     * @throws ClientException ID 为空或文档不存在时抛出
     */
    private KnowledgeDocumentEntity requireDocument(String docId) {
        if (!StringUtils.hasText(docId)) {
            throw new ClientException("文档ID不能为空");
        }
        KnowledgeDocumentEntity document = getById(docId);
        if (document == null) {
            throw new ClientException("文档不存在");
        }
        return document;
    }

    /**
     * 一次性读取上传文件内容，供内容哈希和对象存储共同使用。
     *
     * @param file 上传文件
     * @return 文件全部字节
     * @throws ServiceException 文件读取失败时抛出
     */
    private byte[] readFileContent(MultipartFile file) {
        try {
            return file.getBytes();
        } catch (IOException ex) {
            throw new ServiceException("读取上传文件失败", ex, BaseErrorCode.SERVICE_ERROR);
        }
    }

    /**
     * 取得上传文件名；客户端没有提供时使用固定兜底名。
     *
     * @param file 上传文件
     * @return 非空文件名
     */
    private String resolveFileName(MultipartFile file) {
        String originalFilename = file.getOriginalFilename();
        return StringUtils.hasText(originalFilename) ? originalFilename : "unknown";
    }

    /**
     * 合并调用方元数据和对象存储返回的信息。
     *
     * @param metadata 调用方元数据
     * @param storedFile 已上传文件信息
     * @return 独立的新 Map，不会反向修改请求对象
     */
    private Map<String, Object> resolveMetadata(Map<String, Object> metadata, StoredFileDTO storedFile) {
        Map<String, Object> resolved = metadata == null ? new HashMap<>() : new HashMap<>(metadata);
        resolved.put("storageUrl", storedFile.getUrl());
        resolved.put("detectedType", storedFile.getDetectedType());
        resolved.put("originalFilename", storedFile.getOriginalFilename());
        return resolved;
    }

    /**
     * 字符串有内容时去掉首尾空格，否则使用默认值。
     *
     * @param value 原始值
     * @param defaultValue 兜底值
     * @return 最终值
     */
    private String defaultIfBlank(String value, String defaultValue) {
        return StringUtils.hasText(value) ? value.trim() : defaultValue;
    }

    /**
     * 把可能为空的配置 Map 转换为只读空 Map。
     *
     * @param value 原始配置
     * @return 原 Map 或空 Map
     */
    private Map<String, Object> defaultMap(Map<String, Object> value) {
        return value == null ? Map.of() : value;
    }

    /**
     * 把接口中的策略字符串转换成程序内枚举，并统一不支持策略的错误提示。
     *
     * @param value 策略名称
     * @return 对应切分策略枚举
     * @throws ClientException 策略名称不受支持时抛出
     */
    private ChunkingEnum resolveChunkingEnum(String value) {
        try {
            return ChunkingEnum.fromValue(defaultIfBlank(value, DEFAULT_CHUNK_STRATEGY));
        } catch (IllegalArgumentException ex) {
            throw new ClientException("不支持的分块策略: " + value);
        }
    }

    /**
     * 计算文件内容的 SHA-256 指纹。
     *
     * <p>指纹用于“同知识库、同内容”去重，与用户是否改过文件名无关。</p>
     *
     * @param content 文件内容
     * @return 小写十六进制 SHA-256 字符串
     */
    private String sha256Hex(byte[] content) {
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            return HexFormat.of().formatHex(digest.digest(content));
        } catch (NoSuchAlgorithmException ex) {
            throw new ServiceException("计算文件哈希失败", ex, BaseErrorCode.SERVICE_ERROR);
        }
    }

    /**
     * 尽力删除对象存储文件，但不覆盖原本的业务异常。
     *
     * <p>这是跨数据库和对象存储的补偿动作。删除失败会留下日志供运维清理，但不会让调用方误以为
     * 原本成功的数据库事务失败，也不会掩盖数据库保存失败的真正原因。</p>
     *
     * @param sourceUri 文件访问地址；为空时直接返回
     */
    private void deleteStoredFileQuietly(String sourceUri) {
        if (!StringUtils.hasText(sourceUri)) {
            return;
        }
        try {
            requireFileStorageService().deleteByUrl(sourceUri);
        } catch (Exception ex) {
            log.warn("删除 RustFS 文件失败: {}", sourceUri, ex);
        }
    }

    /**
     * 封装事务外准备好的分块结果，后续可扩展解析统计信息而不改变主流程参数。
     *
     * @param chunks 已完成向量生成和校验的分块
     */
    private record ChunkProcessResult(List<VectorChunk> chunks) {
    }
}
