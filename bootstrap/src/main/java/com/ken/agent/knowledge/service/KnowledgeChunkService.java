package com.ken.agent.knowledge.service;

import com.baomidou.mybatisplus.core.metadata.IPage;
import com.baomidou.mybatisplus.extension.service.IService;
import com.ken.agent.knowledge.controller.request.KnowledgeChunkCreateRequest;
import com.ken.agent.knowledge.controller.request.KnowledgeChunkPageRequest;
import com.ken.agent.knowledge.controller.request.KnowledgeChunkUpdateRequest;
import com.ken.agent.knowledge.dao.entity.KnowledgeChunkEntity;
import com.ken.agent.knowledge.dao.vo.KnowledgeChunkVO;

import java.util.List;

public interface KnowledgeChunkService extends IService<KnowledgeChunkEntity> {

    IPage<KnowledgeChunkVO> pageChunks(String docId, KnowledgeChunkPageRequest requestParam);

    KnowledgeChunkVO createChunk(String docId, KnowledgeChunkCreateRequest requestParam);

    KnowledgeChunkVO updateChunk(String docId, String chunkId, KnowledgeChunkUpdateRequest requestParam);

    void enableChunk(String docId, String chunkId, boolean enabled);

    void deleteChunk(String docId, String chunkId);

    void deleteByDocumentId(String docId);

    List<KnowledgeChunkVO> listByDocumentId(String docId);
}
