package com.ken.agent.knowledge.service.impl;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.ken.agent.core.chunk.VectorChunk;
import com.ken.agent.framework.exception.ServiceException;
import org.junit.jupiter.api.Test;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.jdbc.core.ParameterizedPreparedStatementSetter;

import java.sql.PreparedStatement;
import java.util.List;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.ArgumentMatchers.same;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verifyNoInteractions;
import static org.mockito.Mockito.verify;

class KnowledgeVectorServiceImplTests {

    private static final int EMBEDDING_DIMENSIONS = 1536;

    @Test
    void indexDocumentChunksWritesOneJdbcBatch() throws Exception {
        JdbcTemplate jdbcTemplate = mock(JdbcTemplate.class);
        ObjectMapper objectMapper = new ObjectMapper();
        KnowledgeVectorServiceImpl service = new KnowledgeVectorServiceImpl(jdbcTemplate, objectMapper);
        List<VectorChunk> chunks = List.of(
                vectorChunk("chunk-1", 0, "第一段", 0.1f),
                vectorChunk("chunk-2", 1, "第二段", 0.2f));

        service.indexDocumentChunks("kb-1", "doc-1", chunks);

        @SuppressWarnings({"rawtypes", "unchecked"})
        org.mockito.ArgumentCaptor<ParameterizedPreparedStatementSetter<VectorChunk>> setterCaptor =
                (org.mockito.ArgumentCaptor) org.mockito.ArgumentCaptor.forClass(
                        ParameterizedPreparedStatementSetter.class);
        org.mockito.ArgumentCaptor<String> sqlCaptor = org.mockito.ArgumentCaptor.forClass(String.class);
        verify(jdbcTemplate).batchUpdate(
                sqlCaptor.capture(),
                same(chunks),
                eq(chunks.size()),
                setterCaptor.capture());
        assertThat(sqlCaptor.getValue())
                .contains("INSERT INTO rag.t_knowledge_vector")
                .contains("?::jsonb")
                .contains("?::vector");

        PreparedStatement preparedStatement = mock(PreparedStatement.class);
        setterCaptor.getValue().setValues(preparedStatement, chunks.getFirst());

        verify(preparedStatement).setString(1, "chunk-1");
        verify(preparedStatement).setString(2, "第一段");
        org.mockito.ArgumentCaptor<String> metadataCaptor = org.mockito.ArgumentCaptor.forClass(String.class);
        verify(preparedStatement).setString(eq(3), metadataCaptor.capture());
        Map<String, Object> metadata = objectMapper.readValue(
                metadataCaptor.getValue(), new TypeReference<>() {
                });
        assertThat(metadata)
                .containsEntry("kbId", "kb-1")
                .containsEntry("documentId", "doc-1")
                .containsEntry("chunkId", "chunk-1")
                .containsEntry("chunkIndex", 0);

        org.mockito.ArgumentCaptor<String> vectorCaptor = org.mockito.ArgumentCaptor.forClass(String.class);
        verify(preparedStatement).setString(eq(4), vectorCaptor.capture());
        assertThat(vectorCaptor.getValue()).startsWith("[0.1,0.0").endsWith("]");
    }

    @Test
    void upsertChunkWritesOneJdbcUpdate() {
        JdbcTemplate jdbcTemplate = mock(JdbcTemplate.class);
        KnowledgeVectorServiceImpl service = new KnowledgeVectorServiceImpl(
                jdbcTemplate, new ObjectMapper());
        VectorChunk chunk = vectorChunk("chunk-1", 0, "第一段", 0.1f);

        service.upsertChunk("kb-1", "doc-1", chunk);

        org.mockito.ArgumentCaptor<String> sqlCaptor = org.mockito.ArgumentCaptor.forClass(String.class);
        verify(jdbcTemplate).update(
                sqlCaptor.capture(),
                eq("chunk-1"),
                eq("第一段"),
                anyString(),
                anyString());
        assertThat(sqlCaptor.getValue())
                .contains("INSERT INTO rag.t_knowledge_vector")
                .contains("ON CONFLICT (id) DO UPDATE");
    }

    @Test
    void rejectsEmbeddingWithWrongDimensionsBeforeDatabaseWrite() {
        JdbcTemplate jdbcTemplate = mock(JdbcTemplate.class);
        KnowledgeVectorServiceImpl service = new KnowledgeVectorServiceImpl(
                jdbcTemplate, new ObjectMapper());
        VectorChunk chunk = VectorChunk.builder()
                .chunkId("chunk-1")
                .index(0)
                .content("第一段")
                .embedding(new float[]{0.1f})
                .build();

        assertThatThrownBy(() -> service.upsertChunk("kb-1", "doc-1", chunk))
                .isInstanceOf(com.ken.agent.framework.exception.ServiceException.class)
                .hasMessageContaining("1536");
        verifyNoInteractions(jdbcTemplate);
    }

    @Test
    void preservesJsonSerializationCauseForTroubleshooting() throws Exception {
        JdbcTemplate jdbcTemplate = mock(JdbcTemplate.class);
        ObjectMapper objectMapper = mock(ObjectMapper.class);
        JsonProcessingException serializationFailure = new JsonProcessingException("bad metadata") {
        };
        org.mockito.Mockito.when(objectMapper.writeValueAsString(any()))
                .thenThrow(serializationFailure);
        KnowledgeVectorServiceImpl service = new KnowledgeVectorServiceImpl(jdbcTemplate, objectMapper);

        assertThatThrownBy(() -> service.upsertChunk(
                "kb-1", "doc-1", vectorChunk("chunk-1", 0, "第一段", 0.1f)))
                .isInstanceOf(ServiceException.class)
                .hasCause(serializationFailure);
        verifyNoInteractions(jdbcTemplate);
    }

    @Test
    void deleteOperationsUseExplicitJdbcSql() {
        JdbcTemplate jdbcTemplate = mock(JdbcTemplate.class);
        KnowledgeVectorServiceImpl service = new KnowledgeVectorServiceImpl(
                jdbcTemplate, new ObjectMapper());

        service.deleteDocumentVectors("doc-1");
        service.deleteChunkVector("chunk-1");

        verify(jdbcTemplate).update(
                "DELETE FROM rag.t_knowledge_vector WHERE metadata ->> 'documentId' = ?",
                "doc-1");
        verify(jdbcTemplate).update(
                "DELETE FROM rag.t_knowledge_vector WHERE id = ?",
                "chunk-1");
    }

    private VectorChunk vectorChunk(String chunkId, int index, String content, float firstValue) {
        float[] embedding = new float[EMBEDDING_DIMENSIONS];
        embedding[0] = firstValue;
        return VectorChunk.builder()
                .chunkId(chunkId)
                .index(index)
                .content(content)
                .embedding(embedding)
                .build();
    }
}
