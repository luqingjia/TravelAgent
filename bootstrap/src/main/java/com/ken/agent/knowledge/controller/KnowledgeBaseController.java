package com.ken.agent.knowledge.controller;

import com.baomidou.mybatisplus.core.metadata.IPage;
import com.ken.agent.core.chunk.ChunkingEnum;
import com.ken.agent.framework.result.Result;
import com.ken.agent.knowledge.controller.request.KnowledgeBaseCreateRequest;
import com.ken.agent.knowledge.controller.request.KnowledgeBasePageRequest;
import com.ken.agent.knowledge.controller.request.KnowledgeBaseUpdateRequest;
import com.ken.agent.knowledge.dao.vo.KnowledgeBaseVO;
import com.ken.agent.knowledge.service.KnowledgeBaseService;
import lombok.RequiredArgsConstructor;
import org.springframework.web.bind.annotation.DeleteMapping;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.PutMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.util.Arrays;
import java.util.List;
import java.util.Map;

@RestController
@RequestMapping("/api/knowledge/bases")
@RequiredArgsConstructor
public class KnowledgeBaseController {

    private final KnowledgeBaseService knowledgeBaseService;

    @PostMapping
    public Result<KnowledgeBaseVO> create(@RequestBody KnowledgeBaseCreateRequest requestParam) {
        return Result.success(knowledgeBaseService.createKnowledgeBase(requestParam));
    }

    @PutMapping("/{kb-id}")
    public Result<KnowledgeBaseVO> update(@PathVariable("kb-id") String kbId,
                                          @RequestBody KnowledgeBaseUpdateRequest requestParam) {
        return Result.success(knowledgeBaseService.updateKnowledgeBase(kbId, requestParam));
    }

    @DeleteMapping("/{kb-id}")
    public Result<Void> delete(@PathVariable("kb-id") String kbId) {
        knowledgeBaseService.deleteKnowledgeBase(kbId);
        return Result.success();
    }

    @GetMapping("/{kb-id}")
    public Result<KnowledgeBaseVO> get(@PathVariable("kb-id") String kbId) {
        return Result.success(knowledgeBaseService.getKnowledgeBase(kbId));
    }

    @GetMapping
    public Result<IPage<KnowledgeBaseVO>> page(KnowledgeBasePageRequest requestParam) {
        return Result.success(knowledgeBaseService.pageKnowledgeBases(requestParam));
    }

    @GetMapping("/chunk-strategies")
    public Result<List<Map<String, Object>>> listChunkStrategies() {
        List<Map<String, Object>> strategies = Arrays.stream(ChunkingEnum.values())
                .filter(ChunkingEnum::isVisible)
                .map(strategy -> Map.<String, Object>of(
                        "value", strategy.getValue(),
                        "label", strategy.getLabel(),
                        "defaultConfig", strategy.getDefaultConfig()))
                .toList();
        return Result.success(strategies);
    }
}
