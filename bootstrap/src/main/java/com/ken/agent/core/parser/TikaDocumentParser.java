package com.ken.agent.core.parser;

import lombok.extern.slf4j.Slf4j;
import org.apache.tika.Tika;
import org.apache.tika.parser.pdf.PDFParserConfig;
import org.springframework.stereotype.Component;

import java.io.ByteArrayInputStream;
import java.io.InputStream;
import java.util.Map;

@Component
@Slf4j
public class TikaDocumentParser implements DocumentParser {

    private static final Tika TIKA=new Tika();

    static {
        PDFParserConfig pdfParserConfig = new PDFParserConfig();
        pdfParserConfig.setExtractInlineImages(false);
        pdfParserConfig.setExtractUniqueInlineImagesOnly(false);
    }

    @Override
    public ParseResult parse(byte[] content, String mimeType, Map<String, Object> options) {
        if (content==null|| content.length == 0) {
            return ParseResult.ofText("");
        }

        try(ByteArrayInputStream stream = new ByteArrayInputStream(content)) {

        }catch (Exception e){
            log.error("Tika 解析失败，MIME 类型: {}", mimeType, e);
            throw new ServiceException("文档解析失败: " + e.getMessage());
        }


        return DocumentParser.super.parse(content, mimeType, options);
    }

    @Override
    public String extractText(InputStream stream, String fileName) {
        return DocumentParser.super.extractText(stream, fileName);
    }

    @Override
    public boolean supports(String mimeType) {
        return DocumentParser.super.supports(mimeType);
    }

    @Override
    public String getParserType() {
        return "";
    }
}
