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

@Service
@RequiredArgsConstructor
public class KnowledgeChunkServiceImpl extends ServiceImpl<KnowledgeChunkMapper, KnowledgeChunkEntity>
        implements KnowledgeChunkService {

    private static final short ENABLED = 1;
    private static final short DISABLED = 0;

    private final KnowledgeDocumentMapper knowledgeDocumentMapper;
    private final ChunkEmbeddingService chunkEmbeddingService;
    private final KnowledgeVectorService knowledgeVectorService;

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

        if (DocumentStatus.COMPLETED.getCode().equals(document.getStatus())) {
            VectorChunk vectorChunk = toVectorChunk(chunk);
            chunkEmbeddingService.embeddingText(List.of(vectorChunk));
            knowledgeVectorService.upsertChunk(document.getKbId(), docId, vectorChunk);
        }
        return KnowledgeChunkVO.fromEntity(chunk);
    }

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

    @Override
    @Transactional(rollbackFor = Exception.class)
    public void deleteByDocumentId(String docId) {
        if (!StringUtils.hasText(docId)) {
            return;
        }
        remove(Wrappers.lambdaQuery(KnowledgeChunkEntity.class)
                .eq(KnowledgeChunkEntity::getDocumentId, docId));
    }

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

    private KnowledgeDocumentEntity requireMutableDocument(String docId) {
        KnowledgeDocumentEntity document = requireDocument(docId);
        if (DocumentStatus.PROCESSING.getCode().equals(document.getStatus())) {
            throw new ClientException("文档正在切分处理中，暂不支持修改 Chunk");
        }
        return document;
    }

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

    private int nextChunkIndex(String docId) {
        KnowledgeChunkEntity latest = getOne(Wrappers.lambdaQuery(KnowledgeChunkEntity.class)
                .eq(KnowledgeChunkEntity::getDocumentId, docId)
                .orderByDesc(KnowledgeChunkEntity::getChunkIndex)
                .last("LIMIT 1"));
        return latest == null || latest.getChunkIndex() == null ? 0 : latest.getChunkIndex() + 1;
    }

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
