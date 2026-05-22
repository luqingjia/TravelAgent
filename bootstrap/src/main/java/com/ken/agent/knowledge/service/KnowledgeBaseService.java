package com.ken.agent.knowledge.service;

import com.baomidou.mybatisplus.core.metadata.IPage;
import com.baomidou.mybatisplus.extension.service.IService;
import com.ken.agent.knowledge.controller.request.KnowledgeBaseCreateRequest;
import com.ken.agent.knowledge.controller.request.KnowledgeBasePageRequest;
import com.ken.agent.knowledge.controller.request.KnowledgeBaseUpdateRequest;
import com.ken.agent.knowledge.dao.entity.KnowledgeBaseEntity;
import com.ken.agent.knowledge.dao.vo.KnowledgeBaseVO;

public interface KnowledgeBaseService extends IService<KnowledgeBaseEntity> {

    KnowledgeBaseVO createKnowledgeBase(KnowledgeBaseCreateRequest requestParam);

    KnowledgeBaseVO updateKnowledgeBase(String kbId, KnowledgeBaseUpdateRequest requestParam);

    void deleteKnowledgeBase(String kbId);

    KnowledgeBaseVO getKnowledgeBase(String kbId);

    IPage<KnowledgeBaseVO> pageKnowledgeBases(KnowledgeBasePageRequest requestParam);
}
