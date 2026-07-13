package com.ken.agent.knowledge;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.ken.agent.core.chunk.VectorChunk;
import com.ken.agent.knowledge.service.impl.KnowledgeVectorServiceImpl;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.core.io.FileSystemResource;
import org.springframework.dao.DuplicateKeyException;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.jdbc.datasource.DataSourceTransactionManager;
import org.springframework.jdbc.datasource.DriverManagerDataSource;
import org.springframework.jdbc.datasource.init.ResourceDatabasePopulator;
import org.springframework.transaction.support.TransactionTemplate;
import org.testcontainers.containers.PostgreSQLContainer;
import org.testcontainers.junit.jupiter.Container;
import org.testcontainers.junit.jupiter.Testcontainers;
import org.testcontainers.utility.DockerImageName;

import javax.sql.DataSource;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

@Testcontainers(disabledWithoutDocker = true)
class KnowledgePgvectorIntegrationTests {

    private static final int EMBEDDING_DIMENSIONS = 1536;

    @Container
    private static final PostgreSQLContainer<?> POSTGRES = new PostgreSQLContainer<>(
            DockerImageName.parse("pgvector/pgvector:pg16")
                    .asCompatibleSubstituteFor("postgres"))
            .withDatabaseName("travel_agent_test")
            .withUsername("test")
            .withPassword("test");

    private JdbcTemplate jdbcTemplate;
    private KnowledgeVectorServiceImpl vectorService;
    private DataSource dataSource;

    @BeforeEach
    void setUp() {
        dataSource = new DriverManagerDataSource(
                POSTGRES.getJdbcUrl(), POSTGRES.getUsername(), POSTGRES.getPassword());
        new ResourceDatabasePopulator(
                new FileSystemResource("../resources/database/rag.sql"))
                .execute(dataSource);
        jdbcTemplate = new JdbcTemplate(dataSource);
        vectorService = new KnowledgeVectorServiceImpl(jdbcTemplate, new ObjectMapper());
    }

    @Test
    void writesBatchIntoARealPgvectorColumn() {
        vectorService.indexDocumentChunks("kb-1", "doc-1", List.of(
                vectorChunk("chunk-1", 0, "第一段", 0.1f),
                vectorChunk("chunk-2", 1, "第二段", 0.2f)));

        Integer count = jdbcTemplate.queryForObject(
                "SELECT count(*) FROM rag.t_knowledge_vector", Integer.class);
        Integer dimensions = jdbcTemplate.queryForObject(
                "SELECT vector_dims(embedding) FROM rag.t_knowledge_vector WHERE id = ?",
                Integer.class,
                "chunk-1");
        String documentId = jdbcTemplate.queryForObject(
                "SELECT metadata ->> 'documentId' FROM rag.t_knowledge_vector WHERE id = ?",
                String.class,
                "chunk-1");

        assertThat(count).isEqualTo(2);
        assertThat(dimensions).isEqualTo(EMBEDDING_DIMENSIONS);
        assertThat(documentId).isEqualTo("doc-1");
    }

    @Test
    void vectorReplacementRollsBackTogetherWhenTheTransactionFails() {
        insertChunk("old-chunk", "doc-1", "旧内容");
        vectorService.upsertChunk("kb-1", "doc-1", vectorChunk("old-chunk", 0, "旧内容", 0.1f));
        TransactionTemplate transaction = new TransactionTemplate(
                new DataSourceTransactionManager(dataSource));

        assertThatThrownBy(() -> transaction.executeWithoutResult(status -> {
            jdbcTemplate.update("DELETE FROM rag.t_knowledge_chunk WHERE document_id = ?", "doc-1");
            vectorService.deleteDocumentVectors("doc-1");
            insertChunk("new-chunk", "doc-1", "新内容");
            vectorService.upsertChunk(
                    "kb-1", "doc-1", vectorChunk("new-chunk", 0, "新内容", 0.2f));
            throw new IllegalStateException("模拟分块替换中途失败");
        })).isInstanceOf(IllegalStateException.class);

        assertThat(chunkIds()).containsExactly("old-chunk");
        assertThat(vectorIds()).containsExactly("old-chunk");
    }

    @Test
    void duplicateRuleOnlyBlocksTheSameContentInsideTheSameKnowledgeBase() {
        insertDocument("doc-1", "kb-1", "行程.pdf", "hash-a");

        assertThatThrownBy(() -> insertDocument("doc-2", "kb-1", "副本.pdf", "hash-a"))
                .isInstanceOf(DuplicateKeyException.class);

        insertDocument("doc-3", "kb-2", "副本.pdf", "hash-a");
        insertDocument("doc-4", "kb-1", "行程.pdf", "hash-b");

        Integer count = jdbcTemplate.queryForObject(
                "SELECT count(*) FROM rag.t_knowledge_document", Integer.class);
        assertThat(count).isEqualTo(3);
    }

    private void insertDocument(String id, String kbId, String fileName, String contentHash) {
        jdbcTemplate.update("""
                INSERT INTO rag.t_knowledge_document
                    (id, kb_id, title, source_type, file_name, content_hash)
                VALUES (?, ?, ?, 'file', ?, ?)
                """, id, kbId, fileName, fileName, contentHash);
    }

    private void insertChunk(String id, String documentId, String content) {
        jdbcTemplate.update("""
                INSERT INTO rag.t_knowledge_chunk
                    (id, kb_id, document_id, chunk_index, content)
                VALUES (?, 'kb-1', ?, 0, ?)
                """, id, documentId, content);
    }

    private List<String> chunkIds() {
        return jdbcTemplate.queryForList(
                "SELECT id FROM rag.t_knowledge_chunk ORDER BY id", String.class);
    }

    private List<String> vectorIds() {
        return jdbcTemplate.queryForList(
                "SELECT id FROM rag.t_knowledge_vector ORDER BY id", String.class);
    }

    private VectorChunk vectorChunk(String id, int index, String content, float firstValue) {
        float[] embedding = new float[EMBEDDING_DIMENSIONS];
        embedding[0] = firstValue;
        return VectorChunk.builder()
                .chunkId(id)
                .index(index)
                .content(content)
                .embedding(embedding)
                .build();
    }
}
