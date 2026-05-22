package com.ken.agent.knowledge.controller;

import com.baomidou.mybatisplus.core.metadata.IPage;
import com.ken.agent.framework.result.Result;
import com.ken.agent.knowledge.controller.request.KnowledgeChunkCreateRequest;
import com.ken.agent.knowledge.controller.request.KnowledgeChunkPageRequest;
import com.ken.agent.knowledge.controller.request.KnowledgeChunkUpdateRequest;
import com.ken.agent.knowledge.dao.vo.KnowledgeChunkVO;
import com.ken.agent.knowledge.service.KnowledgeChunkService;
import lombok.RequiredArgsConstructor;
import org.springframework.web.bind.annotation.DeleteMapping;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PatchMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.PutMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/api/knowledge/documents/{doc-id}/chunks")
@RequiredArgsConstructor
public class KnowledgeChunkController {

    private final KnowledgeChunkService knowledgeChunkService;

    @GetMapping
    public Result<IPage<KnowledgeChunkVO>> page(@PathVariable("doc-id") String docId,
                                                KnowledgeChunkPageRequest requestParam) {
        return Result.success(knowledgeChunkService.pageChunks(docId, requestParam));
    }

    @PostMapping
    public Result<KnowledgeChunkVO> create(@PathVariable("doc-id") String docId,
                                           @RequestBody KnowledgeChunkCreateRequest requestParam) {
        return Result.success(knowledgeChunkService.createChunk(docId, requestParam));
    }

    @PutMapping("/{chunk-id}")
    public Result<KnowledgeChunkVO> update(@PathVariable("doc-id") String docId,
                                           @PathVariable("chunk-id") String chunkId,
                                           @RequestBody KnowledgeChunkUpdateRequest requestParam) {
        return Result.success(knowledgeChunkService.updateChunk(docId, chunkId, requestParam));
    }

    @PatchMapping("/{chunk-id}/enable")
    public Result<Void> enable(@PathVariable("doc-id") String docId,
                               @PathVariable("chunk-id") String chunkId,
                               @RequestParam("value") boolean enabled) {
        knowledgeChunkService.enableChunk(docId, chunkId, enabled);
        return Result.success();
    }

    @DeleteMapping("/{chunk-id}")
    public Result<Void> delete(@PathVariable("doc-id") String docId,
                               @PathVariable("chunk-id") String chunkId) {
        knowledgeChunkService.deleteChunk(docId, chunkId);
        return Result.success();
    }
}
