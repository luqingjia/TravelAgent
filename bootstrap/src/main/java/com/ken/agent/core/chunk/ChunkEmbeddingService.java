package com.ken.agent.core.chunk;

import com.ken.agent.framework.exception.ClientException;
import lombok.RequiredArgsConstructor;
import org.springframework.ai.embedding.EmbeddingModel;
import org.springframework.stereotype.Service;

import java.util.List;

@Service
@RequiredArgsConstructor
public class ChunkEmbeddingService {
    private final EmbeddingModel embeddingModel;

    public void embeddingText(List<VectorChunk> chunks) {
        if (chunks == null || chunks.isEmpty()) {
            return;
        }

        if (chunks.stream()
                .allMatch(vectorChunk ->
                        vectorChunk.getEmbedding() != null && vectorChunk.getEmbedding().length > 0)) {
            return;
        }

        //        获取文本进行嵌入
        List<String> contentList = chunks.stream().map(VectorChunk::getContent).toList();
        for (int i = 0; i < contentList.size(); i++) {
            String content = contentList.get(i);
            if (content == null || content.isBlank()) {
                throw new ClientException("待嵌入文本不能为空，索引：" + i);
            }
        }

        List<float[]> embeds = embeddingModel.embed(contentList);

        if (embeds.isEmpty() || embeds.size() != chunks.size()) {
            throw new ClientException("嵌入结果大小不匹配");
        }

        for (int i = 0; i < chunks.size(); i++) {
            float[] vector = embeds.get(i);
            if (vector == null || vector.length == 0) {
                throw new ClientException("嵌入结果缺失，索引：" + i);
            }
            chunks.get(i).setEmbedding(vector);
        }
    }
}
