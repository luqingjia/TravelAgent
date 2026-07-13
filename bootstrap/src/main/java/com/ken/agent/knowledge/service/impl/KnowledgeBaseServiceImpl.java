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

/**
 * 知识库的具体业务实现。
 *
 * <p>可以把知识库理解成装文档的文件夹：这个类只管文件夹的创建、修改、查询和删除。
 * 删除前会先确认文件夹已经清空，避免文档还在、所属知识库却没有了。</p>
 */
@Service
@RequiredArgsConstructor
public class KnowledgeBaseServiceImpl extends ServiceImpl<KnowledgeBaseMapper, KnowledgeBaseEntity>
        implements KnowledgeBaseService {

    private static final String DEFAULT_TYPE = "travel";
    private static final String DEFAULT_VISIBILITY = "private";
    private static final String DEFAULT_STATUS = "active";

    private final KnowledgeDocumentMapper knowledgeDocumentMapper;

    /**
     * {@inheritDoc}
     *
     * <p>先做必填和重名检查，再补齐类型、可见范围、状态这些默认值，最后一次性保存。</p>
     */
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

        // 调用方没填写的通用属性在这里统一补默认值，避免 Controller 或其他入口各补一套。
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

    /**
     * {@inheritDoc}
     *
     * <p>这是“局部修改”：只覆盖调用方明确传入的字段，未传字段继续保留数据库原值。</p>
     */
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

    /**
     * {@inheritDoc}
     *
     * <p>删除前先数一下知识库中的有效文档。只要还有一篇文档，就要求先清理文档，
     * 这样不会制造没有归属的业务数据。</p>
     */
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

    /** {@inheritDoc} */
    @Override
    public KnowledgeBaseVO getKnowledgeBase(String kbId) {
        return KnowledgeBaseVO.fromEntity(requireKnowledgeBase(kbId));
    }

    /**
     * {@inheritDoc}
     *
     * <p>所有筛选条件都是可选的，最终按更新时间倒序，让最近维护的知识库排在前面。</p>
     */
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

    /**
     * 统一校验知识库 ID 并取得实体。
     *
     * <p>增删改查都走同一个入口，错误提示就不会因为调用的方法不同而忽左忽右。</p>
     *
     * @param kbId 知识库 ID
     * @return 已存在的知识库实体
     * @throws ClientException ID 为空或知识库不存在时抛出
     */
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

    /**
     * 字符串有内容时去掉首尾空格，否则使用默认值。
     *
     * @param value 调用方传入的值
     * @param defaultValue 兜底值
     * @return 最终可保存的字符串
     */
    private String defaultIfBlank(String value, String defaultValue) {
        return StringUtils.hasText(value) ? value.trim() : defaultValue;
    }
}
