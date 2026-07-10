package com.ken.agent.knowledge.service.impl;

import com.ken.agent.core.chunk.ChunkEmbeddingService;
import com.ken.agent.core.chunk.ChunkingStrategy;
import com.ken.agent.core.chunk.ChunkingStrategyFactory;
import com.ken.agent.core.chunk.VectorChunk;
import com.ken.agent.core.parser.DocumentParser;
import com.ken.agent.core.parser.DocumentParserSelector;
import com.ken.agent.framework.exception.ClientException;
import com.ken.agent.framework.exception.ServiceException;
import com.ken.agent.framework.storage.FileStorageService;
import com.ken.agent.framework.storage.config.S3StorageProperties;
import com.ken.agent.framework.storage.dto.StoredFileDTO;
import com.ken.agent.knowledge.config.KnowledgeDocumentUploadProperties;
import com.ken.agent.knowledge.dao.entity.KnowledgeBaseEntity;
import com.ken.agent.knowledge.dao.entity.KnowledgeDocumentEntity;
import com.ken.agent.knowledge.dao.mapper.KnowledgeBaseMapper;
import com.ken.agent.knowledge.dao.mapper.KnowledgeDocumentMapper;
import com.ken.agent.knowledge.service.KnowledgeChunkService;
import com.ken.agent.knowledge.service.KnowledgeVectorService;
import com.ken.agent.knowledge.service.support.KnowledgeDocumentUploadPolicy;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.ObjectProvider;
import org.springframework.dao.DataAccessResourceFailureException;
import org.springframework.dao.DuplicateKeyException;
import org.springframework.mock.web.MockMultipartFile;
import org.springframework.test.util.ReflectionTestUtils;
import org.springframework.transaction.support.TransactionOperations;
import org.springframework.transaction.TransactionStatus;
import org.springframework.util.unit.DataSize;

import java.io.ByteArrayInputStream;
import java.io.InputStream;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.function.Consumer;
import java.util.stream.Stream;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.doAnswer;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.verifyNoInteractions;
import static org.mockito.Mockito.when;

class KnowledgeDocumentServiceImplTests {

    @Test
    void successfulUploadStaysPendingAndDoesNotStartChunking() {
        Fixture fixture = new Fixture(Set.of("txt"));
        fixture.stubStoredFile();
        when(fixture.documentMapper.insert(any(KnowledgeDocumentEntity.class))).thenReturn(1);

        var result = fixture.service.upload("kb-1", null, textFile());

        assertThat(result.getStatus()).isEqualTo("pending");
        assertThat(result.getChunkCount()).isZero();
        assertThat(result.getContentHash()).hasSize(64);
        verify(fixture.documentMapper, never()).tryMarkProcessing(anyString());
        verifyNoInteractions(
                fixture.parserSelector,
                fixture.chunkingStrategyFactory,
                fixture.chunkEmbeddingService,
                fixture.knowledgeChunkService,
                fixture.knowledgeVectorService);
    }

    @Test
    void rejectsDisallowedFileBeforeCallingObjectStorage() {
        Fixture fixture = new Fixture(Set.of("pdf"));
        MockMultipartFile file = new MockMultipartFile(
                "file", "run.exe", "application/octet-stream", new byte[]{1});

        assertThatThrownBy(() -> fixture.service.upload("kb-1", null, file))
                .isInstanceOf(ClientException.class)
                .hasMessageContaining("文件类型");
        verifyNoInteractions(fixture.storageService);
    }

    @Test
    void rejectsDuplicateContentBeforeCallingObjectStorage() {
        Fixture fixture = new Fixture(Set.of("txt"));
        when(fixture.documentMapper.existsActiveByKbIdAndContentHash(
                eq("kb-1"), anyString())).thenReturn(true);
        MockMultipartFile file = textFile();

        assertThatThrownBy(() -> fixture.service.upload("kb-1", null, file))
                .isInstanceOf(ClientException.class)
                .hasMessageContaining("重复");
        verifyNoInteractions(fixture.storageService);
    }

    @Test
    void deletesUploadedObjectWhenDocumentInsertFails() {
        Fixture fixture = new Fixture(Set.of("txt"));
        fixture.stubStoredFile();
        when(fixture.documentMapper.insert(any(KnowledgeDocumentEntity.class)))
                .thenThrow(new DataAccessResourceFailureException("database unavailable"));

        assertThatThrownBy(() -> fixture.service.upload("kb-1", null, textFile()))
                .isInstanceOf(DataAccessResourceFailureException.class);
        verify(fixture.storageService).deleteByUrl("s3://knowledge/doc-1");
    }

    @Test
    void compensationFailureDoesNotHideTheOriginalDatabaseFailure() {
        Fixture fixture = new Fixture(Set.of("txt"));
        fixture.stubStoredFile();
        DataAccessResourceFailureException databaseFailure =
                new DataAccessResourceFailureException("database unavailable");
        when(fixture.documentMapper.insert(any(KnowledgeDocumentEntity.class)))
                .thenThrow(databaseFailure);
        org.mockito.Mockito.doThrow(new IllegalStateException("storage unavailable"))
                .when(fixture.storageService).deleteByUrl("s3://knowledge/doc-1");

        assertThatThrownBy(() -> fixture.service.upload("kb-1", null, textFile()))
                .isSameAs(databaseFailure);
    }

    @Test
    void reportsConcurrentUniqueConflictAsDuplicateAndDeletesUploadedObject() {
        Fixture fixture = new Fixture(Set.of("txt"));
        fixture.stubStoredFile();
        when(fixture.documentMapper.insert(any(KnowledgeDocumentEntity.class)))
                .thenThrow(new DuplicateKeyException("uk_knowledge_document_kb_hash_active"));

        assertThatThrownBy(() -> fixture.service.upload("kb-1", null, textFile()))
                .isInstanceOf(ClientException.class)
                .hasMessageContaining("重复");
        verify(fixture.storageService).deleteByUrl("s3://knowledge/doc-1");
    }

    @Test
    void deletesUploadedObjectWhenDocumentInsertUpdatesNoRows() {
        Fixture fixture = new Fixture(Set.of("txt"));
        fixture.stubStoredFile();
        when(fixture.documentMapper.insert(any(KnowledgeDocumentEntity.class))).thenReturn(0);

        assertThatThrownBy(() -> fixture.service.upload("kb-1", null, textFile()))
                .isInstanceOf(ServiceException.class)
                .hasMessageContaining("文档记录");
        verify(fixture.storageService).deleteByUrl("s3://knowledge/doc-1");
    }

    @Test
    void rejectsSecondProcessingRequestBeforeReadingStoredFile() {
        Fixture fixture = new Fixture(Set.of("txt"));
        when(fixture.documentMapper.selectById("doc-1"))
                .thenReturn(KnowledgeDocumentEntity.builder()
                        .id("doc-1")
                        .kbId("kb-1")
                        .status("failed")
                        .chunkStrategy("structure_aware")
                        .build());
        when(fixture.documentMapper.tryMarkProcessing("doc-1")).thenReturn(0);

        assertThatThrownBy(() -> fixture.service.startChunk("doc-1", null))
                .isInstanceOf(ClientException.class)
                .hasMessageContaining("正在");
        verifyNoInteractions(fixture.storageService);
    }

    @Test
    void processingFailureAddsLastErrorWithoutLosingExistingMetadata() {
        Fixture fixture = new Fixture(Set.of("txt"));
        fixture.stubDocument(Map.of("storageUrl", "s3://knowledge/doc-1"));
        DocumentParser parser = fixture.stubParserAndChunks("正文", List.of());
        when(parser.extractText(any(InputStream.class), eq("guide.txt")))
                .thenThrow(new IllegalStateException("解析器失败"));

        assertThatThrownBy(() -> fixture.service.startChunk("doc-1", null))
                .isInstanceOf(ServiceException.class)
                .hasMessageContaining("解析器失败");

        org.mockito.ArgumentCaptor<KnowledgeDocumentEntity> updateCaptor =
                org.mockito.ArgumentCaptor.forClass(KnowledgeDocumentEntity.class);
        verify(fixture.documentMapper).updateById(updateCaptor.capture());
        assertThat(updateCaptor.getValue().getStatus()).isEqualTo("failed");
        assertThat(updateCaptor.getValue().getMetadata())
                .containsEntry("storageUrl", "s3://knowledge/doc-1")
                .containsEntry("lastError", "解析器失败");
    }

    @Test
    void successfulRetryClearsLastErrorAndKeepsExistingMetadata() {
        Fixture fixture = new Fixture(Set.of("txt"));
        fixture.stubDocument(Map.of(
                "storageUrl", "s3://knowledge/doc-1",
                "lastError", "上次失败"));
        VectorChunk chunk = vectorChunk(new float[1536]);
        fixture.stubParserAndChunks("正文", List.of(chunk));
        fixture.executeTransactions();

        fixture.service.startChunk("doc-1", null);

        org.mockito.ArgumentCaptor<KnowledgeDocumentEntity> updateCaptor =
                org.mockito.ArgumentCaptor.forClass(KnowledgeDocumentEntity.class);
        verify(fixture.documentMapper).updateById(updateCaptor.capture());
        assertThat(updateCaptor.getValue().getStatus()).isEqualTo("completed");
        assertThat(updateCaptor.getValue().getMetadata())
                .containsEntry("storageUrl", "s3://knowledge/doc-1")
                .doesNotContainKey("lastError");
        verify(fixture.knowledgeChunkService).deletePhysicallyByDocumentId("doc-1");
    }

    @Test
    void emptyChunkResultFailsBeforeReplacementTransaction() {
        Fixture fixture = new Fixture(Set.of("txt"));
        fixture.stubDocument(Map.of("storageUrl", "s3://knowledge/doc-1"));
        fixture.stubParserAndChunks("正文", List.of());

        assertThatThrownBy(() -> fixture.service.startChunk("doc-1", null))
                .isInstanceOf(ServiceException.class)
                .hasMessageContaining("分块");
        verifyNoInteractions(fixture.transactionOperations);
        verifyNoInteractions(fixture.knowledgeChunkService, fixture.knowledgeVectorService);
    }

    @Test
    void emptyParsedTextFailsBeforeReplacementTransaction() {
        Fixture fixture = new Fixture(Set.of("txt"));
        fixture.stubDocument(Map.of("storageUrl", "s3://knowledge/doc-1"));
        fixture.stubParserAndChunks("   ", List.of());

        assertThatThrownBy(() -> fixture.service.startChunk("doc-1", null))
                .isInstanceOf(ServiceException.class)
                .hasMessageContaining("解析结果为空");
        verifyNoInteractions(fixture.transactionOperations);
        verifyNoInteractions(fixture.knowledgeChunkService, fixture.knowledgeVectorService);
    }

    @Test
    void wrongEmbeddingDimensionsFailBeforeReplacementTransaction() {
        Fixture fixture = new Fixture(Set.of("txt"));
        fixture.stubDocument(Map.of("storageUrl", "s3://knowledge/doc-1"));
        fixture.stubParserAndChunks("正文", List.of(vectorChunk(new float[]{0.1f})));

        assertThatThrownBy(() -> fixture.service.startChunk("doc-1", null))
                .isInstanceOf(ServiceException.class)
                .hasMessageContaining("1536");
        verifyNoInteractions(fixture.transactionOperations);
        verifyNoInteractions(fixture.knowledgeChunkService, fixture.knowledgeVectorService);
    }

    private static VectorChunk vectorChunk(float[] embedding) {
        return VectorChunk.builder()
                .chunkId("chunk-1")
                .index(0)
                .content("正文")
                .embedding(embedding)
                .build();
    }

    private static MockMultipartFile textFile() {
        return new MockMultipartFile(
                "file", "guide.txt", "text/plain", "内容".getBytes(java.nio.charset.StandardCharsets.UTF_8));
    }

    private static final class Fixture {

        private final KnowledgeDocumentMapper documentMapper = mock(KnowledgeDocumentMapper.class);
        private final FileStorageService storageService = mock(FileStorageService.class);
        private final KnowledgeChunkService knowledgeChunkService = mock(KnowledgeChunkService.class);
        private final KnowledgeVectorService knowledgeVectorService = mock(KnowledgeVectorService.class);
        private final DocumentParserSelector parserSelector = mock(DocumentParserSelector.class);
        private final ChunkingStrategyFactory chunkingStrategyFactory = mock(ChunkingStrategyFactory.class);
        private final ChunkEmbeddingService chunkEmbeddingService = mock(ChunkEmbeddingService.class);
        private final TransactionOperations transactionOperations = mock(TransactionOperations.class);
        private final KnowledgeDocumentServiceImpl service;

        @SuppressWarnings("unchecked")
        private Fixture(Set<String> allowedExtensions) {
            KnowledgeBaseMapper knowledgeBaseMapper = mock(KnowledgeBaseMapper.class);
            when(knowledgeBaseMapper.selectById("kb-1"))
                    .thenReturn(KnowledgeBaseEntity.builder().id("kb-1").build());
            ObjectProvider<FileStorageService> storageProvider = mock(ObjectProvider.class);
            when(storageProvider.getIfAvailable()).thenReturn(storageService);
            ObjectProvider<S3StorageProperties> s3PropertiesProvider = mock(ObjectProvider.class);
            when(s3PropertiesProvider.stream()).thenReturn(Stream.empty());
            KnowledgeDocumentUploadProperties properties = new KnowledgeDocumentUploadProperties();
            properties.setAllowedExtensions(allowedExtensions);
            properties.setMaxSize(DataSize.ofMegabytes(1));
            KnowledgeDocumentUploadPolicy uploadPolicy = new KnowledgeDocumentUploadPolicy(properties);
            service = new KnowledgeDocumentServiceImpl(
                    knowledgeBaseMapper,
                    knowledgeChunkService,
                    knowledgeVectorService,
                    parserSelector,
                    chunkingStrategyFactory,
                    chunkEmbeddingService,
                    storageProvider,
                    s3PropertiesProvider,
                    transactionOperations,
                    uploadPolicy);
            ReflectionTestUtils.setField(service, "baseMapper", documentMapper);
        }

        private void stubDocument(Map<String, Object> metadata) {
            KnowledgeDocumentEntity document = KnowledgeDocumentEntity.builder()
                    .id("doc-1")
                    .kbId("kb-1")
                    .title("指南")
                    .sourceUri("s3://knowledge/doc-1")
                    .fileName("guide.txt")
                    .fileType("txt")
                    .status("failed")
                    .chunkStrategy("structure_aware")
                    .metadata(metadata)
                    .build();
            when(documentMapper.selectById("doc-1")).thenReturn(document);
            when(documentMapper.tryMarkProcessing("doc-1")).thenReturn(1);
        }

        private DocumentParser stubParserAndChunks(String text, List<VectorChunk> chunks) {
            when(storageService.openStream("s3://knowledge/doc-1"))
                    .thenReturn(new ByteArrayInputStream(new byte[]{1}));
            DocumentParser parser = mock(DocumentParser.class);
            when(parserSelector.selectByFile("guide.txt", "txt")).thenReturn(parser);
            when(parser.extractText(any(InputStream.class), eq("guide.txt"))).thenReturn(text);
            ChunkingStrategy strategy = mock(ChunkingStrategy.class);
            when(chunkingStrategyFactory.getChunkingStrategy(any())).thenReturn(strategy);
            when(strategy.chunk(eq(text), any())).thenReturn(chunks);
            return parser;
        }

        @SuppressWarnings("unchecked")
        private void executeTransactions() {
            doAnswer(invocation -> {
                Consumer<TransactionStatus> callback = invocation.getArgument(0);
                callback.accept(mock(TransactionStatus.class));
                return null;
            }).when(transactionOperations).executeWithoutResult(any());
        }

        private void stubStoredFile() {
            when(storageService.reliableUpload(
                    eq("knowledge"),
                    any(InputStream.class),
                    eq((long) "内容".getBytes(java.nio.charset.StandardCharsets.UTF_8).length),
                    eq("guide.txt"),
                    eq("text/plain")))
                    .thenReturn(StoredFileDTO.builder()
                            .url("s3://knowledge/doc-1")
                            .detectedType("txt")
                            .size((long) "内容".getBytes(java.nio.charset.StandardCharsets.UTF_8).length)
                            .originalFilename("guide.txt")
                            .build());
        }
    }
}
