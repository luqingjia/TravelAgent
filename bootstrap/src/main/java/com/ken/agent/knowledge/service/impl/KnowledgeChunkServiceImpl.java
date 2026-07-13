package com.ken.agent.knowledge.service.impl;

import cn.hutool.core.util.IdUtil;
import com.baomidou.mybatisplus.core.conditions.query.LambdaQueryWrapper;
import com.baomidou.mybatisplus.core.metadata.IPage;
import com.baomidou.mybatisplus.core.toolkit.Wrappers;
import com.baomidou.mybatisplus.extension.plugins.pagination.Page;
import com.baomidou.mybatisplus.extension.service.impl.ServiceImpl;
import com.ken.agent.core.chunk.ChunkEmbeddingService;
import com.ken.agent.core.chunk.VectorChunk;
import com.ken.agent.framework.exception.ClientException;
import com.ken.agent.knowledge.controller.request.KnowledgeChunkCreateRequest;
import com.ken.agent.knowledge.controller.request.KnowledgeChunkPageRequest;
import com.ken.agent.knowledge.controller.request.KnowledgeChunkUpdateRequest;
import com.ken.agent.knowledge.dao.entity.KnowledgeChunkEntity;
import com.ken.agent.knowledge.dao.entity.KnowledgeDocumentEntity;
import com.ken.agent.knowledge.dao.mapper.KnowledgeChunkMapper;
import com.ken.agent.knowledge.dao.mapper.KnowledgeDocumentMapper;
import com.ken.agent.knowledge.dao.vo.KnowledgeChunkVO;
import com.ken.agent.knowledge.enums.DocumentStatus;
import com.ken.agent.knowledge.service.KnowledgeChunkService;
import com.ken.agent.knowledge.service.KnowledgeVectorService;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;
import org.springframework.util.StringUtils;

import java.util.List;
import java.util.Map;

/**
 * 文档分块的具体业务实现。
 *
 * <p>这个类处理用户对分块的手工维护。它不能只改分块表：文档上的分块数量以及 pgvector
 * 里的向量也必须跟着变化，所以相关写操作都放在同一事务中。任何一步失败，整次修改都会回滚，
 * 不会出现“页面文字已经变了，但搜索仍命中旧内容”的情况。</p>
 */
@Service
@RequiredArgsConstructor
public class KnowledgeChunkServiceImpl extends ServiceImpl<KnowledgeChunkMapper, KnowledgeChunkEntity>
        implements KnowledgeChunkService {

    private static final short ENABLED = 1;
    private static final short DISABLED = 0;

    private final KnowledgeDocumentMapper knowledgeDocumentMapper;
    private final ChunkEmbeddingService chunkEmbeddingService;
    private final KnowledgeVectorService knowledgeVectorService;

    /** {@inheritDoc} */
    @Override
    public IPage<KnowledgeChunkVO> pageChunks(String docId, KnowledgeChunkPageRequest requestParam) {
        requireDocument(docId);
        KnowledgeChunkPageRequest request = requestParam == null ? new KnowledgeChunkPageRequest() : requestParam;
        LambdaQueryWrapper<KnowledgeChunkEntity> wrapper = Wrappers.lambdaQuery(KnowledgeChunkEntity.class)
                .eq(KnowledgeChunkEntity::getDocumentId, docId)
                .eq(request.getEnabled() != null, KnowledgeChunkEntity::getEnabled, request.getEnabled())
                .orderByAsc(KnowledgeChunkEntity::getChunkIndex);
        return page(new Page<>(request.getCurrent(), request.getSize()), wrapper)
                .convert(KnowledgeChunkVO::fromEntity);
    }

    /**
     * {@inheritDoc}
     *
     * <p>先保存分块并增加文档计数。如果文档已经进入可检索状态，还要当场生成向量并写入
     * pgvector；这些数据库操作共享当前事务。</p>
     */
    @Override
    @Transactional(rollbackFor = Exception.class)
    public KnowledgeChunkVO createChunk(String docId, KnowledgeChunkCreateRequest requestParam) {
        KnowledgeDocumentEntity document = requireMutableDocument(docId);
        if (requestParam == null || !StringUtils.hasText(requestParam.getContent())) {
            throw new ClientException("Chunk 内容不能为空");
        }

        int chunkIndex = requestParam.getChunkIndex() != null ? requestParam.getChunkIndex() : nextChunkIndex(docId);
        KnowledgeChunkEntity chunk = KnowledgeChunkEntity.builder()
                .id(IdUtil.getSnowflakeNextIdStr())
                .kbId(document.getKbId())
                .documentId(docId)
                .chunkIndex(chunkIndex)
                .content(requestParam.getContent())
                .tokenCount(requestParam.getContent().length())
                .charCount(requestParam.getContent().length())
                .metadata(requestParam.getMetadata())
                .enabled(ENABLED)
                .build();
        save(chunk);
        knowledgeDocumentMapper.update(null, Wrappers.lambdaUpdate(KnowledgeDocumentEntity.class)
                .eq(KnowledgeDocumentEntity::getId, docId)
                .setSql("chunk_count = COALESCE(chunk_count, 0) + 1"));

        // 未完成的文档以后还会整体切分，不必提前为手工分块生成向量；已完成文档则要立即同步。
        if (DocumentStatus.COMPLETED.getCode().equals(document.getStatus())) {
            VectorChunk vectorChunk = toVectorChunk(chunk);
            chunkEmbeddingService.embeddingText(List.of(vectorChunk));
            knowledgeVectorService.upsertChunk(document.getKbId(), docId, vectorChunk);
        }
        return KnowledgeChunkVO.fromEntity(chunk);
    }

    /**
     * {@inheritDoc}
     *
     * <p>修改已启用且可检索的分块时重新算向量。因为向量以分块 ID 做主键，写入会覆盖旧向量。</p>
     */
    @Override
    @Transactional(rollbackFor = Exception.class)
    public KnowledgeChunkVO updateChunk(String docId, String chunkId, KnowledgeChunkUpdateRequest requestParam) {
        KnowledgeDocumentEntity document = requireMutableDocument(docId);
        KnowledgeChunkEntity chunk = requireChunk(docId, chunkId);
        if (requestParam == null || !StringUtils.hasText(requestParam.getContent())) {
            throw new ClientException("Chunk 内容不能为空");
        }

        chunk.setContent(requestParam.getContent());
        chunk.setCharCount(requestParam.getContent().length());
        chunk.setTokenCount(requestParam.getContent().length());
        if (requestParam.getMetadata() != null) {
            chunk.setMetadata(requestParam.getMetadata());
        }
        updateById(chunk);

        if (DocumentStatus.COMPLETED.getCode().equals(document.getStatus())
                && chunk.getEnabled() != null
                && chunk.getEnabled() == ENABLED) {
            VectorChunk vectorChunk = toVectorChunk(chunk);
            chunkEmbeddingService.embeddingText(List.of(vectorChunk));
            knowledgeVectorService.upsertChunk(document.getKbId(), docId, vectorChunk);
        }
        return KnowledgeChunkVO.fromEntity(chunk);
    }

    /**
     * {@inheritDoc}
     *
     * <p>停用相当于让搜索暂时看不见该分块，所以直接删除向量；重新启用时再按当前文字生成向量。</p>
     */
    @Override
    @Transactional(rollbackFor = Exception.class)
    public void enableChunk(String docId, String chunkId, boolean enabled) {
        KnowledgeDocumentEntity document = requireMutableDocument(docId);
        KnowledgeChunkEntity chunk = requireChunk(docId, chunkId);
        short target = enabled ? ENABLED : DISABLED;
        if (chunk.getEnabled() != null && chunk.getEnabled() == target) {
            return;
        }
        chunk.setEnabled(target);
        updateById(chunk);
        if (!enabled) {
            knowledgeVectorService.deleteChunkVector(chunkId);
            return;
        }
        if (DocumentStatus.COMPLETED.getCode().equals(document.getStatus())) {
            VectorChunk vectorChunk = toVectorChunk(chunk);
            chunkEmbeddingService.embeddingText(List.of(vectorChunk));
            knowledgeVectorService.upsertChunk(document.getKbId(), docId, vectorChunk);
        }
    }

    /**
     * {@inheritDoc}
     *
     * <p>分块逻辑删除、文档计数减一、向量删除必须作为一件事成功或失败。</p>
     */
    @Override
    @Transactional(rollbackFor = Exception.class)
    public void deleteChunk(String docId, String chunkId) {
        requireMutableDocument(docId);
        requireChunk(docId, chunkId);
        removeById(chunkId);
        knowledgeDocumentMapper.update(null, Wrappers.lambdaUpdate(KnowledgeDocumentEntity.class)
                .eq(KnowledgeDocumentEntity::getId, docId)
                .setSql("chunk_count = CASE WHEN COALESCE(chunk_count, 0) > 0 THEN chunk_count - 1 ELSE 0 END"));
        knowledgeVectorService.deleteChunkVector(chunkId);
    }

    /** {@inheritDoc} */
    @Override
    @Transactional(rollbackFor = Exception.class)
    public void deleteByDocumentId(String docId) {
        if (!StringUtils.hasText(docId)) {
            return;
        }
        remove(Wrappers.lambdaQuery(KnowledgeChunkEntity.class)
                .eq(KnowledgeChunkEntity::getDocumentId, docId));
    }

    /**
     * {@inheritDoc}
     *
     * <p>这里故意绕过 MyBatis-Plus 的逻辑删除，只供重新切分的短事务调用。</p>
     */
    @Override
    @Transactional(rollbackFor = Exception.class)
    public void deletePhysicallyByDocumentId(String docId) {
        if (!StringUtils.hasText(docId)) {
            return;
        }
        getBaseMapper().deletePhysicallyByDocumentId(docId);
    }

    /** {@inheritDoc} */
    @Override
    public List<KnowledgeChunkVO> listByDocumentId(String docId) {
        requireDocument(docId);
        return list(Wrappers.lambdaQuery(KnowledgeChunkEntity.class)
                .eq(KnowledgeChunkEntity::getDocumentId, docId)
                .orderByAsc(KnowledgeChunkEntity::getChunkIndex))
                .stream()
                .map(KnowledgeChunkVO::fromEntity)
                .toList();
    }

    /**
     * 取得允许修改的文档。
     *
     * <p>切分任务正在工作时禁止用户同时改分块，否则新切分结果和手工修改会互相覆盖。</p>
     *
     * @param docId 文档 ID
     * @return 当前不处于处理中的文档
     * @throws ClientException 文档不存在或正在处理中时抛出
     */
    private KnowledgeDocumentEntity requireMutableDocument(String docId) {
        KnowledgeDocumentEntity document = requireDocument(docId);
        if (DocumentStatus.PROCESSING.getCode().equals(document.getStatus())) {
            throw new ClientException("文档正在切分处理中，暂不支持修改 Chunk");
        }
        return document;
    }

    /**
     * 校验文档 ID 并读取文档。
     *
     * @param docId 文档 ID
     * @return 文档实体
     * @throws ClientException ID 为空或文档不存在时抛出
     */
    private KnowledgeDocumentEntity requireDocument(String docId) {
        if (!StringUtils.hasText(docId)) {
            throw new ClientException("文档ID不能为空");
        }
        KnowledgeDocumentEntity document = knowledgeDocumentMapper.selectById(docId);
        if (document == null) {
            throw new ClientException("文档不存在");
        }
        return document;
    }

    /**
     * 读取分块并确认它确实属于当前文档。
     *
     * <p>只按分块 ID 查询不够安全，否则调用方可能拿另一个文档的分块 ID 来修改数据。</p>
     *
     * @param docId 当前文档 ID
     * @param chunkId 分块 ID
     * @return 属于当前文档的分块实体
     * @throws ClientException 分块不存在或归属不匹配时抛出
     */
    private KnowledgeChunkEntity requireChunk(String docId, String chunkId) {
        if (!StringUtils.hasText(chunkId)) {
            throw new ClientException("Chunk ID不能为空");
        }
        KnowledgeChunkEntity chunk = getById(chunkId);
        if (chunk == null || !docId.equals(chunk.getDocumentId())) {
            throw new ClientException("Chunk 不存在或不属于该文档");
        }
        return chunk;
    }

    /**
     * 计算追加分块时默认使用的顺序号。
     *
     * @param docId 文档 ID
     * @return 当前最大顺序号加一；没有分块时返回零
     */
    private int nextChunkIndex(String docId) {
        KnowledgeChunkEntity latest = getOne(Wrappers.lambdaQuery(KnowledgeChunkEntity.class)
                .eq(KnowledgeChunkEntity::getDocumentId, docId)
                .orderByDesc(KnowledgeChunkEntity::getChunkIndex)
                .last("LIMIT 1"));
        return latest == null || latest.getChunkIndex() == null ? 0 : latest.getChunkIndex() + 1;
    }

    /**
     * 把数据库分块转换成向量服务认识的对象。
     *
     * <p>这里只整理文字和元数据，真正的向量值由 {@link ChunkEmbeddingService} 随后填入。</p>
     *
     * @param chunk 数据库分块
     * @return 等待生成向量的分块对象
     */
    private VectorChunk toVectorChunk(KnowledgeChunkEntity chunk) {
        Map<String, Object> metadata = chunk.getMetadata() == null ? Map.of() : chunk.getMetadata();
        return VectorChunk.builder()
                .chunkId(chunk.getId())
                .index(chunk.getChunkIndex())
                .content(chunk.getContent())
                .metadata(metadata)
                .build();
    }
}
