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
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.beans.factory.ObjectProvider;
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

@Slf4j
@Service
@RequiredArgsConstructor
public class KnowledgeDocumentServiceImpl extends ServiceImpl<KnowledgeDocumentMapper, KnowledgeDocumentEntity>
        implements KnowledgeDocumentService {

    private static final String DEFAULT_BUCKET = "knowledge";
    private static final String DEFAULT_LANGUAGE = "zh";
    private static final String DEFAULT_CHUNK_STRATEGY = "structure_aware";

    private final KnowledgeBaseMapper knowledgeBaseMapper;
    private final KnowledgeChunkService knowledgeChunkService;
    private final KnowledgeVectorService knowledgeVectorService;
    private final DocumentParserSelector parserSelector;
    private final ChunkingStrategyFactory chunkingStrategyFactory;
    private final ChunkEmbeddingService chunkEmbeddingService;
    private final ObjectProvider<FileStorageService> fileStorageServiceProvider;
    private final ObjectProvider<S3StorageProperties> s3StoragePropertiesProvider;
    private final TransactionOperations transactionOperations;

    @Override
    public KnowledgeDocumentVO upload(String kbId, KnowledgeDocumentUploadRequest requestParam, MultipartFile file) {
        requireKnowledgeBase(kbId);
        if (file == null || file.isEmpty()) {
            throw new ClientException("上传文件不能为空");
        }

        KnowledgeDocumentUploadRequest request = requestParam == null
                ? KnowledgeDocumentUploadRequest.builder().build()
                : requestParam;
        SourceType sourceType = SourceType.normalize(request.getSourceType());
        if (sourceType != SourceType.FILE) {
            throw new ClientException("当前仅支持文件上传类型");
        }

        byte[] content = readFileContent(file);
        String fileName = resolveFileName(file);
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
                .contentHash(sha256Hex(content))
                .language(defaultIfBlank(request.getLanguage(), DEFAULT_LANGUAGE))
                .status(DocumentStatus.PENDING.getCode())
                .chunkCount(0)
                .chunkStrategy(defaultIfBlank(request.getChunkStrategy(), DEFAULT_CHUNK_STRATEGY))
                .chunkConfig(request.getChunkConfig())
                .metadata(resolveMetadata(request.getMetadata(), storedFile))
                .build();
        save(entity);
        return KnowledgeDocumentVO.fromEntity(entity);
    }

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

        markStatus(docId, DocumentStatus.PROCESSING, null);
        long startTime = System.currentTimeMillis();
        try {
            ChunkProcessResult processResult = extractChunkAndEmbed(document, chunkingEnum, chunkConfig);
            persistChunksAndVectors(document, chunkingEnum, chunkConfig, processResult.chunks());
            log.info("文档切分完成, docId={}, chunks={}, duration={}ms",
                    docId, processResult.chunks().size(), System.currentTimeMillis() - startTime);
            return getDocument(docId);
        } catch (Exception ex) {
            log.error("文档切分失败, docId={}", docId, ex);
            markStatus(docId, DocumentStatus.FAILED, ex.getMessage());
            throw ex instanceof ClientException ? (ClientException) ex
                    : new ServiceException("文档切分失败: " + ex.getMessage(), ex, BaseErrorCode.SERVICE_ERROR);
        }
    }

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

    @Override
    public KnowledgeDocumentVO getDocument(String docId) {
        return KnowledgeDocumentVO.fromEntity(requireDocument(docId));
    }

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

        ChunkingOptions options = chunkingEnum.createOptions(chunkConfig);
        ChunkingStrategy strategy = chunkingStrategyFactory.getChunkingStrategy(chunkingEnum);
        List<VectorChunk> chunks = strategy.chunk(text, options);
        for (VectorChunk chunk : chunks) {
            Map<String, Object> metadata = new HashMap<>(chunk.getMetadata() == null ? Map.of() : chunk.getMetadata());
            metadata.put("title", document.getTitle());
            metadata.put("fileName", document.getFileName());
            chunk.setMetadata(metadata);
        }
        chunkEmbeddingService.embeddingText(chunks);
        return new ChunkProcessResult(chunks);
    }

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

        transactionOperations.executeWithoutResult(status -> {
            knowledgeChunkService.deleteByDocumentId(document.getId());
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
                    .build();
            updateById(update);
        });
    }

    private void markStatus(String docId, DocumentStatus status, String errorMessage) {
        KnowledgeDocumentEntity update = KnowledgeDocumentEntity.builder()
                .id(docId)
                .status(status.getCode())
                .build();
        if (StringUtils.hasText(errorMessage)) {
            Map<String, Object> metadata = new HashMap<>();
            metadata.put("lastError", errorMessage);
            update.setMetadata(metadata);
        }
        updateById(update);
    }

    private StoredFileDTO uploadToRustFs(byte[] content, String fileName, String contentType) {
        FileStorageService storageService = requireFileStorageService();
        String bucketName = s3StoragePropertiesProvider.stream()
                .map(S3StorageProperties::getBucketName)
                .filter(StringUtils::hasText)
                .findFirst()
                .orElse(DEFAULT_BUCKET);
        return storageService.reliableUpload(bucketName, new java.io.ByteArrayInputStream(content), content.length, fileName, contentType);
    }

    private FileStorageService requireFileStorageService() {
        FileStorageService storageService = fileStorageServiceProvider.getIfAvailable();
        if (storageService == null) {
            throw new ServiceException("RustFS 文件存储未启用，请配置 agent.storage.s3.enabled=true");
        }
        return storageService;
    }

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

    private byte[] readFileContent(MultipartFile file) {
        try {
            return file.getBytes();
        } catch (IOException ex) {
            throw new ServiceException("读取上传文件失败", ex, BaseErrorCode.SERVICE_ERROR);
        }
    }

    private String resolveFileName(MultipartFile file) {
        String originalFilename = file.getOriginalFilename();
        return StringUtils.hasText(originalFilename) ? originalFilename : "unknown";
    }

    private Map<String, Object> resolveMetadata(Map<String, Object> metadata, StoredFileDTO storedFile) {
        Map<String, Object> resolved = metadata == null ? new HashMap<>() : new HashMap<>(metadata);
        resolved.put("storageUrl", storedFile.getUrl());
        resolved.put("detectedType", storedFile.getDetectedType());
        resolved.put("originalFilename", storedFile.getOriginalFilename());
        return resolved;
    }

    private String defaultIfBlank(String value, String defaultValue) {
        return StringUtils.hasText(value) ? value.trim() : defaultValue;
    }

    private Map<String, Object> defaultMap(Map<String, Object> value) {
        return value == null ? Map.of() : value;
    }

    private ChunkingEnum resolveChunkingEnum(String value) {
        try {
            return ChunkingEnum.fromValue(defaultIfBlank(value, DEFAULT_CHUNK_STRATEGY));
        } catch (IllegalArgumentException ex) {
            throw new ClientException("不支持的分块策略: " + value);
        }
    }

    private String sha256Hex(byte[] content) {
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            return HexFormat.of().formatHex(digest.digest(content));
        } catch (NoSuchAlgorithmException ex) {
            throw new ServiceException("计算文件哈希失败", ex, BaseErrorCode.SERVICE_ERROR);
        }
    }

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

    private record ChunkProcessResult(List<VectorChunk> chunks) {
    }
}
