package com.ken.agent.knowledge.controller.request;

import lombok.Data;

@Data
public class KnowledgeBasePageRequest {

    private long current = 1;

    private long size = 10;

    private String keyword;

    private String type;

    private String status;
}
