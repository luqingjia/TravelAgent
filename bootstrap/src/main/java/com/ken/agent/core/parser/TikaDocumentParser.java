package com.ken.agent.core.parser;

import com.ken.framework.exception.ServiceException;
import lombok.extern.slf4j.Slf4j;
import org.apache.tika.Tika;
import org.apache.tika.parser.AutoDetectParser;
import org.apache.tika.parser.ParseContext;
import org.apache.tika.parser.pdf.PDFParserConfig;
import org.springframework.stereotype.Component;

import java.io.ByteArrayInputStream;
import java.io.InputStream;
import java.util.Map;

@Component
@Slf4j
public class TikaDocumentParser implements DocumentParser {

    private static final Tika TIKA=new Tika();

/*    static {
        PDFParserConfig pdfParserConfig = new PDFParserConfig();
        pdfParserConfig.setExtractInlineImages(false);
        pdfParserConfig.setExtractUniqueInlineImagesOnly(false);
    }*/

    @Override
    public ParseResult parse(byte[] content, String mimeType, Map<String, Object> options) {
        if (content==null|| content.length == 0) {
            return ParseResult.ofText("");
        }

        try(ByteArrayInputStream stream = new ByteArrayInputStream(content)) {
            String parsedToString = TIKA.parseToString(stream);
            String cleanedString = TextCleanupUtil.cleanup(parsedToString);
            return ParseResult.ofText(cleanedString);
        }catch (Exception e){
            log.error("Tika 解析失败，MIME 类型: {}", mimeType, e);
            throw new ServiceException("文档解析失败: " + e.getMessage());
        }
    }

    @Override
    public String extractText(InputStream stream, String fileName) {
        if (stream==null){
            return "";
        }

        try {
            String parsedToString = TIKA.parseToString(stream);
            return TextCleanupUtil.cleanup(parsedToString);
        } catch (Exception e) {
            log.error("从文件中提取文本内容失败: {}", fileName, e);
            throw new ServiceException("解析文件失败: " + fileName);
        }

    }

    @Override
    public boolean supports(String mimeType) {
        return mimeType !=null && !mimeType.startsWith("text/markdown");
    }

    @Override
    public String getParserType() {
        return ParserType.TIKA.getType();
    }
}
