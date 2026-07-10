package com.ken.agent.knowledge.service.support;

import com.ken.agent.framework.exception.ClientException;
import com.ken.agent.knowledge.config.KnowledgeDocumentUploadProperties;
import org.junit.jupiter.api.Test;
import org.springframework.mock.web.MockMultipartFile;
import org.springframework.util.unit.DataSize;
import org.springframework.web.multipart.MultipartFile;

import java.util.Set;

import static org.assertj.core.api.Assertions.assertThatCode;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

class KnowledgeDocumentUploadPolicyTests {

    @Test
    void rejectsEmptyFile() {
        KnowledgeDocumentUploadPolicy policy = policy(Set.of("txt"), DataSize.ofMegabytes(1));
        MultipartFile file = new MockMultipartFile(
                "file", "empty.txt", "text/plain", new byte[0]);

        assertThatThrownBy(() -> policy.validate(file))
                .isInstanceOf(ClientException.class)
                .hasMessageContaining("不能为空");
    }

    @Test
    void acceptsConfiguredExtensionIgnoringCase() {
        KnowledgeDocumentUploadPolicy policy = policy(Set.of("pdf"), DataSize.ofMegabytes(1));
        MultipartFile file = new MockMultipartFile(
                "file", "GUIDE.PDF", "application/pdf", new byte[]{1});

        assertThatCode(() -> policy.validate(file)).doesNotThrowAnyException();
    }

    @Test
    void rejectsExtensionOutsideConfiguredAllowList() {
        KnowledgeDocumentUploadPolicy policy = policy(Set.of("pdf"), DataSize.ofMegabytes(1));
        MultipartFile file = new MockMultipartFile(
                "file", "run.exe", "application/octet-stream", new byte[]{1});

        assertThatThrownBy(() -> policy.validate(file))
                .isInstanceOf(ClientException.class)
                .hasMessageContaining("文件类型")
                .hasMessageContaining("exe");
    }

    @Test
    void rejectsFileLargerThanConfiguredLimitBeforeReadingContent() {
        KnowledgeDocumentUploadPolicy policy = policy(Set.of("txt"), DataSize.ofBytes(10));
        MultipartFile file = mock(MultipartFile.class);
        when(file.isEmpty()).thenReturn(false);
        when(file.getOriginalFilename()).thenReturn("guide.txt");
        when(file.getSize()).thenReturn(11L);

        assertThatThrownBy(() -> policy.validate(file))
                .isInstanceOf(ClientException.class)
                .hasMessageContaining("10B");
    }

    private KnowledgeDocumentUploadPolicy policy(Set<String> extensions, DataSize maxSize) {
        KnowledgeDocumentUploadProperties properties = new KnowledgeDocumentUploadProperties();
        properties.setAllowedExtensions(extensions);
        properties.setMaxSize(maxSize);
        return new KnowledgeDocumentUploadPolicy(properties);
    }
}
