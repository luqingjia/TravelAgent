package com.ken.agent.knowledge;

import org.junit.jupiter.api.Test;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;

import static org.assertj.core.api.Assertions.assertThat;

class KnowledgeDatabaseSchemaTests {

    private static final Path DATABASE_DIR = Path.of("..", "resources", "database");

    @Test
    void baselineUses1536DimensionAndHnswCosineIndex() throws IOException {
        String sql = read("rag.sql");

        assertThat(sql)
                .contains("CREATE SCHEMA IF NOT EXISTS rag")
                .contains("vector(1536)")
                .contains("USING hnsw")
                .contains("embedding vector_cosine_ops");
    }

    @Test
    void baselinePreventsDuplicateActiveContentInsideOneKnowledgeBase() throws IOException {
        String sql = read("rag.sql");

        assertThat(sql)
                .contains("CREATE UNIQUE INDEX \"uk_knowledge_document_kb_hash_active\"")
                .contains("\"kb_id\", \"content_hash\"")
                .contains("WHERE deleted = 0 AND content_hash IS NOT NULL");
    }

    @Test
    void upgradeScriptChangesIndexesWithoutDroppingBusinessTables() throws IOException {
        String sql = read("knowledge-ingestion-mvp-upgrade.sql");

        assertThat(sql)
                .contains("ALTER TABLE rag.t_knowledge_vector")
                .contains("vector(1536)")
                .contains("USING hnsw")
                .doesNotContainIgnoringCase("DROP TABLE");
    }

    private String read(String fileName) throws IOException {
        return Files.readString(DATABASE_DIR.resolve(fileName));
    }
}
