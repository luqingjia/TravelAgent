package com.ken.agent.core.chunk;

import java.util.List;

/**
 * 文本分块器核心接口
 * 定义统一的文本分块能力
 */
public interface ChunkingStrategy {
    ChunkingEnum getChunkingStrategyEnum();

    List<VectorChunk> chunk(String text,ChunkingOptions options);
}
