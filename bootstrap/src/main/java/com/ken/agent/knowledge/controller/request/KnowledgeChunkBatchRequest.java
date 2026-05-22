package com.ken.agent.knowledge.controller.request;

import lombok.Data;

import java.util.List;

@Data
public class KnowledgeChunkBatchRequest {

    private List<String> chunkIds;
}
