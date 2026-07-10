package com.ken.agent.knowledge.config;

import org.junit.jupiter.api.Test;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.boot.test.context.runner.ApplicationContextRunner;
import org.springframework.context.annotation.Configuration;
import org.springframework.util.unit.DataSize;

import static org.assertj.core.api.Assertions.assertThat;

class KnowledgeDocumentUploadPropertiesTests {

    private final ApplicationContextRunner contextRunner = new ApplicationContextRunner()
            .withUserConfiguration(TestConfiguration.class);

    @Test
    void bindsAllowedExtensionsAndMaximumSizeFromConfiguration() {
        contextRunner
                .withPropertyValues(
                        "agent.knowledge.document.upload.allowed-extensions=pdf,txt",
                        "agent.knowledge.document.upload.max-size=7MB")
                .run(context -> {
                    KnowledgeDocumentUploadProperties properties =
                            context.getBean(KnowledgeDocumentUploadProperties.class);

                    assertThat(properties.getAllowedExtensions()).containsExactlyInAnyOrder("pdf", "txt");
                    assertThat(properties.getMaxSize()).isEqualTo(DataSize.ofMegabytes(7));
                });
    }

    @Configuration(proxyBeanMethods = false)
    @EnableConfigurationProperties(KnowledgeDocumentUploadProperties.class)
    static class TestConfiguration {
    }
}
