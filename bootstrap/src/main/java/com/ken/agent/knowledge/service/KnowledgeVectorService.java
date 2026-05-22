package com.ken.agent.knowledge.service;

import com.ken.agent.core.chunk.VectorChunk;

import java.util.List;

public interface KnowledgeVectorService {

    void indexDocumentChunks(String kbId, String documentId, List<VectorChunk> chunks);

    void upsertChunk(String kbId, String documentId, VectorChunk chunk);

    void deleteDocumentVectors(String documentId);

    void deleteChunkVector(String chunkId);
}
