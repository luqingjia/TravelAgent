package com.ken.agent.core.chunk;

import lombok.RequiredArgsConstructor;
import org.springframework.ai.embedding.EmbeddingModel;
import org.springframework.stereotype.Service;

// TODO
@Service
@RequiredArgsConstructor
public class ChunkEmbeddingService {
    private EmbeddingModel embeddingModel;

    

}
