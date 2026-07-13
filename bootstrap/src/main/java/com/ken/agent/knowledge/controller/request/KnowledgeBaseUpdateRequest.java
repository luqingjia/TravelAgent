package com.ken.agent.knowledge.controller.request;

import lombok.Data;

import java.util.Map;

@Data
public class KnowledgeBaseUpdateRequest {

    private String name;

    private String description;

    private String type;

    private String visibility;

    private String status;

    private Map<String, Object> metadata;
}
