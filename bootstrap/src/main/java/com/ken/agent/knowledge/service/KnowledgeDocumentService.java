package com.ken.agent.knowledge.service;

import com.baomidou.mybatisplus.core.metadata.IPage;
import com.baomidou.mybatisplus.extension.service.IService;
import com.ken.agent.knowledge.controller.request.KnowledgeDocumentChunkRequest;
import com.ken.agent.knowledge.controller.request.KnowledgeDocumentPageRequest;
import com.ken.agent.knowledge.dao.dto.KnowledgeDocumentUploadRequest;
import com.ken.agent.knowledge.dao.entity.KnowledgeDocumentEntity;
import com.ken.agent.knowledge.dao.vo.KnowledgeDocumentVO;
import org.springframework.web.multipart.MultipartFile;

public interface KnowledgeDocumentService extends IService<KnowledgeDocumentEntity> {
    /**
     * 上传文档
     *
     * @param kbId         知识库 ID
     * @param requestParam 请求对象参数
     * @param file         待上传的文件
     * @return 知识库文档视图对象
     */
    KnowledgeDocumentVO upload(String kbId, KnowledgeDocumentUploadRequest requestParam, MultipartFile file);

    KnowledgeDocumentVO startChunk(String docId, KnowledgeDocumentChunkRequest requestParam);

    void deleteDocument(String docId);

    KnowledgeDocumentVO getDocument(String docId);

    IPage<KnowledgeDocumentVO> pageDocuments(String kbId, KnowledgeDocumentPageRequest requestParam);
}
