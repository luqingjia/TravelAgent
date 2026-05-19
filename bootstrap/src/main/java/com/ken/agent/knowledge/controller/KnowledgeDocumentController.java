package com.ken.agent.knowledge.controller;

import com.ken.agent.framework.result.Result;
import com.ken.agent.knowledge.dao.vo.KnowledgeDocumentVO;
import com.ken.agent.knowledge.service.KnowledgeDocumentService;
import lombok.RequiredArgsConstructor;
import org.springframework.http.MediaType;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.multipart.MultipartFile;

@RestController
@RequestMapping("/api/knowledge/documents")
@RequiredArgsConstructor
public class KnowledgeDocumentController {
    private final KnowledgeDocumentService knowledgeDocumentService;

    @PostMapping(value = "/knowledge-base/{kb-id}/doc/upload",consumes= MediaType.MULTIPART_FORM_DATA_VALUE)
    public Result<KnowledgeDocumentVO> knowledgeBaseDocUpload(@RequestParam("file") MultipartFile file){

        return null;
    }


}
