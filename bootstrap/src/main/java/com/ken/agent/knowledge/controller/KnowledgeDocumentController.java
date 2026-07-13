package com.ken.agent.knowledge.controller;

import com.baomidou.mybatisplus.core.metadata.IPage;
import com.ken.agent.framework.result.Result;
import com.ken.agent.knowledge.controller.request.KnowledgeDocumentChunkRequest;
import com.ken.agent.knowledge.controller.request.KnowledgeDocumentPageRequest;
import com.ken.agent.knowledge.dao.dto.KnowledgeDocumentUploadRequest;
import com.ken.agent.knowledge.dao.vo.KnowledgeDocumentVO;
import com.ken.agent.knowledge.service.KnowledgeDocumentService;
import lombok.RequiredArgsConstructor;
import org.springframework.http.MediaType;
import org.springframework.web.bind.annotation.DeleteMapping;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.ModelAttribute;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestPart;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.multipart.MultipartFile;

@RestController
@RequestMapping("/api/knowledge")
@RequiredArgsConstructor
public class KnowledgeDocumentController {

    private final KnowledgeDocumentService knowledgeDocumentService;

    @PostMapping(value = "/bases/{kb-id}/documents/upload", consumes = MediaType.MULTIPART_FORM_DATA_VALUE)
    public Result<KnowledgeDocumentVO> upload(@PathVariable("kb-id") String kbId,
                                              @RequestPart("file") MultipartFile file,
                                              @ModelAttribute KnowledgeDocumentUploadRequest requestParam) {
        return Result.success(knowledgeDocumentService.upload(kbId, requestParam, file));
    }

    @PostMapping("/documents/{doc-id}/chunk")
    public Result<KnowledgeDocumentVO> startChunk(@PathVariable("doc-id") String docId,
                                                  @RequestBody(required = false) KnowledgeDocumentChunkRequest requestParam) {
        return Result.success(knowledgeDocumentService.startChunk(docId, requestParam));
    }

    @DeleteMapping("/documents/{doc-id}")
    public Result<Void> delete(@PathVariable("doc-id") String docId) {
        knowledgeDocumentService.deleteDocument(docId);
        return Result.success();
    }

    @GetMapping("/documents/{doc-id}")
    public Result<KnowledgeDocumentVO> get(@PathVariable("doc-id") String docId) {
        return Result.success(knowledgeDocumentService.getDocument(docId));
    }

    @GetMapping("/documents/{doc-id}/status")
    public Result<KnowledgeDocumentVO> status(@PathVariable("doc-id") String docId) {
        return Result.success(knowledgeDocumentService.getDocument(docId));
    }

    @GetMapping("/bases/{kb-id}/documents")
    public Result<IPage<KnowledgeDocumentVO>> page(@PathVariable("kb-id") String kbId,
                                                   KnowledgeDocumentPageRequest requestParam) {
        return Result.success(knowledgeDocumentService.pageDocuments(kbId, requestParam));
    }
}
