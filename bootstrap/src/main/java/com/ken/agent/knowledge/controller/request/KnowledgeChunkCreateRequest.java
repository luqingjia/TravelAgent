package com.ken.agent.knowledge.controller.request;

import lombok.Data;

import java.util.HashMap;
import java.util.Map;

@Data
public class KnowledgeChunkCreateRequest {

    private Integer chunkIndex;

    private String content;

    private Map<String, Object> metadata = new HashMap<>();
}
