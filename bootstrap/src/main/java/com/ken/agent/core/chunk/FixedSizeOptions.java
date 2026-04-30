package com.ken.agent.core.chunk;

import java.util.Map;

public record FixedSizeOptions(
        int chunkSize,
        int overlapSize
) implements ChunkingOptions {
    @Override
    public Map<String, Integer> toConfigMap() {
        return Map.of(
                "chunkSize",chunkSize,
                "overlapSize",overlapSize
        );
    }
}
