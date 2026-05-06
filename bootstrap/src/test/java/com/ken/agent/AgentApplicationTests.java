package com.ken.agent;

import com.ken.agent.core.parser.TikaDocumentParser;
import org.junit.jupiter.api.Test;
import org.springframework.ai.embedding.EmbeddingModel;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.test.context.SpringBootTest;

import java.io.FileInputStream;
import java.util.Arrays;
import java.util.List;

@SpringBootTest
class AgentApplicationTests {
    @Autowired
    private TikaDocumentParser tikaDocumentParser;
    @Autowired
    private EmbeddingModel embeddingModel;
    @Value("${spring.ai.openai.embedding.api-key}")
    private String API_KEY;
    @Test
    void getEmbeddingApiKeyTest(){
        System.out.println(this.API_KEY);
    }

    @Test
    void tikaParserTest(){
        try(FileInputStream fileInputStream = new FileInputStream("C:\\Users\\ken\\Desktop\\4.01TEMU-衬衫套装加急-1套（4.01出）-半托.xlsx")){
            String s = tikaDocumentParser.extractText(fileInputStream, "C:\\Users\\ken\\Desktop\\4.01TEMU-衬衫套装加急-1套（4.01出）-半托.xlsx");
            System.out.println(s);
        }catch (Exception e){
            e.printStackTrace();
        }
    }


    @Test
    void embeddingModelTest() {
        List<String> strings = List.of("kenLuQingJia");

        List<float[]> embed = embeddingModel.embed(strings);

        System.out.println("文本数量：" + embed.size());

        if (!embed.isEmpty()) {
            System.out.println("向量维度：" + embed.get(0).length);

            for (int i = 0; i < embed.size(); i++) {
                float[] vector = embed.get(i);

                System.out.println("第 " + i + " 个文本：" + strings.get(i));
                System.out.println("向量维度：" + vector.length);
                System.out.println("完整向量：" + Arrays.toString(vector));
            }
        }
    }

}
