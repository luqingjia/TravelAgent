package com.ken.agent;

import org.junit.jupiter.api.Test;
import org.springframework.boot.test.context.SpringBootTest;

@SpringBootTest(properties = {
        "agent.storage.s3.bucket-name=test-bucket",
        "agent.storage.s3.access-key=test-access-key",
        "agent.storage.s3.secret-key=test-secret-key",
        "spring.ai.openai.chat.api-key=test-chat-key",
        "spring.ai.openai.embedding.api-key=test-embedding-key",
        "spring.datasource.username=test-user",
        "spring.datasource.password=test-password"
})
class AgentApplicationTests {

    @Test
    void contextLoads() {
    }
}
