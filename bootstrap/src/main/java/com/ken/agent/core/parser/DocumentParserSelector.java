package com.ken.agent.core.parser;

import com.ken.agent.framework.exception.ServiceException;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Component;
import org.springframework.util.StringUtils;

import java.util.List;

@Component
@RequiredArgsConstructor
public class DocumentParserSelector {

    private final List<DocumentParser> parsers;

    public DocumentParser select(String parserType) {
        return parsers.stream()
                .filter(parser -> parser.getParserType().equalsIgnoreCase(parserType))
                .findFirst()
                .orElseThrow(() -> new ServiceException("未找到文档解析器: " + parserType));
    }

    public DocumentParser selectByFile(String fileName, String fileType) {
        if (isMarkdown(fileName, fileType)) {
            return select(ParserType.MARKDOWN.getType());
        }
        return select(ParserType.TIKA.getType());
    }

    private boolean isMarkdown(String fileName, String fileType) {
        String extension = StringUtils.getFilenameExtension(fileName);
        if (StringUtils.hasText(extension)) {
            String normalized = extension.trim().toLowerCase();
            if ("md".equals(normalized) || "markdown".equals(normalized) || "txt".equals(normalized)) {
                return true;
            }
        }
        if (!StringUtils.hasText(fileType)) {
            return false;
        }
        String normalized = fileType.trim().toLowerCase();
        return "md".equals(normalized)
                || "markdown".equals(normalized)
                || "txt".equals(normalized)
                || "plain".equals(normalized)
                || "text/markdown".equals(normalized)
                || "text/plain".equals(normalized);
    }
}
