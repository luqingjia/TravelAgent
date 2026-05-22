package com.ken.agent.knowledge.enums;

import com.ken.agent.framework.exception.ClientException;
import lombok.Getter;
import lombok.RequiredArgsConstructor;

@Getter
@RequiredArgsConstructor
public enum SourceType {

    FILE("file");

    private final String value;

    public static SourceType normalize(String value) {
        if (value == null || value.isBlank()) {
            return FILE;
        }
        String normalized = value.trim().toLowerCase();
        for (SourceType sourceType : values()) {
            if (sourceType.value.equals(normalized)) {
                return sourceType;
            }
        }
        throw new ClientException("不支持的文档来源类型: " + value);
    }
}
