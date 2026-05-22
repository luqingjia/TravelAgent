package com.ken.agent.knowledge.service.impl;

import cn.hutool.core.util.IdUtil;
import com.baomidou.mybatisplus.core.conditions.query.LambdaQueryWrapper;
import com.baomidou.mybatisplus.core.metadata.IPage;
import com.baomidou.mybatisplus.core.toolkit.Wrappers;
import com.baomidou.mybatisplus.extension.plugins.pagination.Page;
import com.baomidou.mybatisplus.extension.service.impl.ServiceImpl;
import com.ken.agent.framework.exception.ClientException;
import com.ken.agent.framework.exception.ServiceException;
import com.ken.agent.knowledge.controller.request.KnowledgeBaseCreateRequest;
import com.ken.agent.knowledge.controller.request.KnowledgeBasePageRequest;
import com.ken.agent.knowledge.controller.request.KnowledgeBaseUpdateRequest;
import com.ken.agent.knowledge.dao.entity.KnowledgeBaseEntity;
import com.ken.agent.knowledge.dao.entity.KnowledgeDocumentEntity;
import com.ken.agent.knowledge.dao.mapper.KnowledgeBaseMapper;
import com.ken.agent.knowledge.dao.mapper.KnowledgeDocumentMapper;
import com.ken.agent.knowledge.dao.vo.KnowledgeBaseVO;
import com.ken.agent.knowledge.service.KnowledgeBaseService;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;
import org.springframework.util.StringUtils;

@Service
@RequiredArgsConstructor
public class KnowledgeBaseServiceImpl extends ServiceImpl<KnowledgeBaseMapper, KnowledgeBaseEntity>
        implements KnowledgeBaseService {

    private static final String DEFAULT_TYPE = "travel";
    private static final String DEFAULT_VISIBILITY = "private";
    private static final String DEFAULT_STATUS = "active";

    private final KnowledgeDocumentMapper knowledgeDocumentMapper;

    @Override
    @Transactional(rollbackFor = Exception.class)
    public KnowledgeBaseVO createKnowledgeBase(KnowledgeBaseCreateRequest requestParam) {
        if (requestParam == null || !StringUtils.hasText(requestParam.getName())) {
            throw new ClientException("知识库名称不能为空");
        }
        long count = count(Wrappers.lambdaQuery(KnowledgeBaseEntity.class)
                .eq(KnowledgeBaseEntity::getName, requestParam.getName().trim()));
        if (count > 0) {
            throw new ServiceException("知识库名称已存在：" + requestParam.getName());
        }

        KnowledgeBaseEntity entity = KnowledgeBaseEntity.builder()
                .id(IdUtil.getSnowflakeNextIdStr())
                .name(requestParam.getName().trim())
                .description(requestParam.getDescription())
                .type(defaultIfBlank(requestParam.getType(), DEFAULT_TYPE))
                .ownerUserId(requestParam.getOwnerUserId())
                .visibility(defaultIfBlank(requestParam.getVisibility(), DEFAULT_VISIBILITY))
                .status(DEFAULT_STATUS)
                .metadata(requestParam.getMetadata())
                .build();
        save(entity);
        return KnowledgeBaseVO.fromEntity(entity);
    }

    @Override
    @Transactional(rollbackFor = Exception.class)
    public KnowledgeBaseVO updateKnowledgeBase(String kbId, KnowledgeBaseUpdateRequest requestParam) {
        KnowledgeBaseEntity entity = requireKnowledgeBase(kbId);
        if (requestParam == null) {
            return KnowledgeBaseVO.fromEntity(entity);
        }
        if (StringUtils.hasText(requestParam.getName())) {
            entity.setName(requestParam.getName().trim());
        }
        if (requestParam.getDescription() != null) {
            entity.setDescription(requestParam.getDescription());
        }
        if (StringUtils.hasText(requestParam.getType())) {
            entity.setType(requestParam.getType().trim());
        }
        if (StringUtils.hasText(requestParam.getVisibility())) {
            entity.setVisibility(requestParam.getVisibility().trim());
        }
        if (StringUtils.hasText(requestParam.getStatus())) {
            entity.setStatus(requestParam.getStatus().trim());
        }
        if (requestParam.getMetadata() != null) {
            entity.setMetadata(requestParam.getMetadata());
        }
        updateById(entity);
        return KnowledgeBaseVO.fromEntity(entity);
    }

    @Override
    @Transactional(rollbackFor = Exception.class)
    public void deleteKnowledgeBase(String kbId) {
        KnowledgeBaseEntity entity = requireKnowledgeBase(kbId);
        Long documentCount = knowledgeDocumentMapper.selectCount(Wrappers.lambdaQuery(KnowledgeDocumentEntity.class)
                .eq(KnowledgeDocumentEntity::getKbId, kbId));
        if (documentCount != null && documentCount > 0) {
            throw new ClientException("当前知识库下还有文档，请先删除文档");
        }
        removeById(entity.getId());
    }

    @Override
    public KnowledgeBaseVO getKnowledgeBase(String kbId) {
        return KnowledgeBaseVO.fromEntity(requireKnowledgeBase(kbId));
    }

    @Override
    public IPage<KnowledgeBaseVO> pageKnowledgeBases(KnowledgeBasePageRequest requestParam) {
        KnowledgeBasePageRequest request = requestParam == null ? new KnowledgeBasePageRequest() : requestParam;
        LambdaQueryWrapper<KnowledgeBaseEntity> wrapper = Wrappers.lambdaQuery(KnowledgeBaseEntity.class)
                .like(StringUtils.hasText(request.getKeyword()), KnowledgeBaseEntity::getName, request.getKeyword())
                .eq(StringUtils.hasText(request.getType()), KnowledgeBaseEntity::getType, request.getType())
                .eq(StringUtils.hasText(request.getStatus()), KnowledgeBaseEntity::getStatus, request.getStatus())
                .orderByDesc(KnowledgeBaseEntity::getUpdateTime);
        return page(new Page<>(request.getCurrent(), request.getSize()), wrapper)
                .convert(KnowledgeBaseVO::fromEntity);
    }

    private KnowledgeBaseEntity requireKnowledgeBase(String kbId) {
        if (!StringUtils.hasText(kbId)) {
            throw new ClientException("知识库ID不能为空");
        }
        KnowledgeBaseEntity entity = getById(kbId);
        if (entity == null) {
            throw new ClientException("知识库不存在");
        }
        return entity;
    }

    private String defaultIfBlank(String value, String defaultValue) {
        return StringUtils.hasText(value) ? value.trim() : defaultValue;
    }
}
