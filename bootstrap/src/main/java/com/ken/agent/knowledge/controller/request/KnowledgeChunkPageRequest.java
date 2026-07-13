package com.ken.agent.knowledge.controller.request;

import lombok.Data;

@Data
public class KnowledgeChunkPageRequest {

    private long current = 1;

    private long size = 20;

    private Short enabled;
}
