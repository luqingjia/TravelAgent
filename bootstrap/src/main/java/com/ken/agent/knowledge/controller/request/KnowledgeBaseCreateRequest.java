package com.ken.agent.knowledge.controller.request;

import lombok.Data;

import java.util.HashMap;
import java.util.Map;

@Data
public class KnowledgeBaseCreateRequest {

    private String name;

    private String description;

    private String type = "travel";

    private String ownerUserId;

    private String visibility = "private";

    private Map<String, Object> metadata = new HashMap<>();
}
