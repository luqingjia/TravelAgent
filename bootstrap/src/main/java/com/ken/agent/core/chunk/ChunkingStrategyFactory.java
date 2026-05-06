package com.ken.agent.core.chunk;

import jakarta.annotation.PostConstruct;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Component;

import java.util.*;

// TODO
@Component
@RequiredArgsConstructor
public class ChunkingStrategyFactory {
    //注入所有ChunkingStrategy的实现类
    private final List<ChunkingStrategy> chunkingStrategies;
    private volatile Map<ChunkingEnum, ChunkingStrategy> chunkingStrategyMap=Map.of();

    @PostConstruct
    public void init(){
        EnumMap<ChunkingEnum, ChunkingStrategy> enumMap = new EnumMap<>(ChunkingEnum.class);

        chunkingStrategies.forEach(
                chunker->{
                    ChunkingStrategy put = enumMap.put(chunker.getChunkingStrategyEnum(), chunker);
                    if (put != null) {
                        throw new IllegalStateException(
                                "Duplicate ChunkingStrategy for type: " + put.getChunkingStrategyEnum()
                                        + " (" + put.getClass().getName() + " vs " + put.getClass().getName() + ")"
                        );
                    }
                }
        );

        this.chunkingStrategyMap=Map.copyOf(enumMap);
    }

    public ChunkingStrategy getChunkingStrategy(ChunkingEnum chunkingEnum){
        Objects.requireNonNull(chunkingEnum,"ChunkingEnum cannot be null");
        return Optional.ofNullable(
                chunkingStrategyMap.get(chunkingEnum)
        ).orElseThrow(
                () -> new IllegalStateException("No ChunkingStrategy for type: " + chunkingEnum)
        );
    }

}
