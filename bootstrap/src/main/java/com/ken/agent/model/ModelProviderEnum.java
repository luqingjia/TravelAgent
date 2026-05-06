package com.ken.agent.model;

import lombok.Getter;
import lombok.RequiredArgsConstructor;

@Getter
@RequiredArgsConstructor
public enum ModelProviderEnum {
    /**
     * Ollama 本地模型服务
     */
    OLLAMA("ollama"),

    /**
     * 阿里云百炼大模型平台
     */
    BAI_LIAN("bailian"),

    /**
     * 硅基流动 AI 模型服务
     */
    SILICON_FLOW("siliconflow"),

    /**
     * 空实现，用于测试或占位
     */
    NOOP("noop");

    private final String id;

    public boolean matches(String provider) {
        return provider != null && provider.equalsIgnoreCase(id);
    }

}
