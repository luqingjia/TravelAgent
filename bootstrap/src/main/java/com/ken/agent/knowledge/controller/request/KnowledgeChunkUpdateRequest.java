package com.ken.agent.knowledge.controller.request;

import lombok.Data;

import java.util.Map;

@Data
public class KnowledgeChunkUpdateRequest {

    private String content;

    private Map<String, Object> metadata;
}
