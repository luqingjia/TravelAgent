package com.ken.agent;

import com.ken.agent.core.parser.TikaDocumentParser;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;

import java.io.FileInputStream;
import java.nio.file.Path;

@SpringBootTest
class AgentApplicationTests {
    @Autowired
    private TikaDocumentParser tikaDocumentParser;


    @Test
    void contextLoads() {
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

}
