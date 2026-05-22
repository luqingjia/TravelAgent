package com.ken.agent.knowledge.controller.request;

import lombok.Data;

import java.util.HashMap;
import java.util.Map;

@Data
public class KnowledgeDocumentChunkRequest {

    private String chunkStrategy = "structure_aware";

    private Map<String, Object> chunkConfig = new HashMap<>();
}
