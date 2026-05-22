package com.ken.agent.knowledge.service.impl;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.ken.agent.core.chunk.VectorChunk;
import com.ken.agent.framework.exception.ServiceException;
import com.ken.agent.knowledge.dao.mapper.KnowledgeVectorMapper;
import com.ken.agent.knowledge.service.KnowledgeVectorService;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.util.HashMap;
import java.util.List;
import java.util.Map;

@Service
@RequiredArgsConstructor
public class KnowledgeVectorServiceImpl implements KnowledgeVectorService {

    private final KnowledgeVectorMapper knowledgeVectorMapper;
    private final ObjectMapper objectMapper;

    @Override
    public void indexDocumentChunks(String kbId, String documentId, List<VectorChunk> chunks) {
        if (chunks == null || chunks.isEmpty()) {
            return;
        }
        for (VectorChunk chunk : chunks) {
            upsertChunk(kbId, documentId, chunk);
        }
    }

    @Override
    public void upsertChunk(String kbId, String documentId, VectorChunk chunk) {
        if (chunk == null || chunk.getEmbedding() == null || chunk.getEmbedding().length == 0) {
            throw new ServiceException("Chunk 向量不能为空");
        }
        knowledgeVectorMapper.upsertVector(
                chunk.getChunkId(),
                chunk.getContent(),
                toJson(buildMetadata(kbId, documentId, chunk)),
                toPgVector(chunk.getEmbedding()));
    }

    @Override
    public void deleteDocumentVectors(String documentId) {
        if (documentId == null || documentId.isBlank()) {
            return;
        }
        knowledgeVectorMapper.deleteByDocumentId(documentId);
    }

    @Override
    public void deleteChunkVector(String chunkId) {
        if (chunkId == null || chunkId.isBlank()) {
            return;
        }
        knowledgeVectorMapper.deleteByChunkId(chunkId);
    }

    private Map<String, Object> buildMetadata(String kbId, String documentId, VectorChunk chunk) {
        Map<String, Object> metadata = new HashMap<>();
        if (chunk.getMetadata() != null) {
            metadata.putAll(chunk.getMetadata());
        }
        metadata.put("kbId", kbId);
        metadata.put("documentId", documentId);
        metadata.put("chunkId", chunk.getChunkId());
        metadata.put("chunkIndex", chunk.getIndex());
        return metadata;
    }

    private String toJson(Map<String, Object> metadata) {
        try {
            return objectMapper.writeValueAsString(metadata);
        } catch (JsonProcessingException ex) {
            throw new ServiceException("向量元数据序列化失败");
        }
    }

    private String toPgVector(float[] embedding) {
        StringBuilder builder = new StringBuilder("[");
        for (int i = 0; i < embedding.length; i++) {
            if (i > 0) {
                builder.append(',');
            }
            builder.append(embedding[i]);
        }
        return builder.append(']').toString();
    }
}
