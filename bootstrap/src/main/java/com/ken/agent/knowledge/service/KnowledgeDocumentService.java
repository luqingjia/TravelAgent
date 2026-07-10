package com.ken.agent.knowledge.service;

import com.baomidou.mybatisplus.core.metadata.IPage;
import com.baomidou.mybatisplus.extension.service.IService;
import com.ken.agent.knowledge.controller.request.KnowledgeDocumentChunkRequest;
import com.ken.agent.knowledge.controller.request.KnowledgeDocumentPageRequest;
import com.ken.agent.knowledge.dao.dto.KnowledgeDocumentUploadRequest;
import com.ken.agent.knowledge.dao.entity.KnowledgeDocumentEntity;
import com.ken.agent.knowledge.dao.vo.KnowledgeDocumentVO;
import org.springframework.web.multipart.MultipartFile;

/**
 * 知识库文档业务接口。
 *
 * <p>文档分两步处理：先上传并保存文档记录，再由调用方显式触发切分。这样上传成功后即使
 * 切分失败，也可以查看失败原因并重试，不需要重新上传文件。</p>
 */
public interface KnowledgeDocumentService extends IService<KnowledgeDocumentEntity> {
    /**
     * 上传文档，但不自动切分。
     *
     * <p>服务会检查扩展名、大小和同知识库内容重复，然后先保存文件、再保存文档记录。
     * 如果数据库保存失败，会尽力删除刚上传的文件作为补偿。</p>
     *
     * @param kbId         知识库 ID
     * @param requestParam 请求对象参数
     * @param file         待上传的文件
     * @return 知识库文档视图对象
     * @throws com.ken.agent.framework.exception.ClientException 文件不合规、内容重复或知识库不存在时抛出
     * @throws com.ken.agent.framework.exception.ServiceException 文件存储或数据库保存失败时抛出
     */
    KnowledgeDocumentVO upload(String kbId, KnowledgeDocumentUploadRequest requestParam, MultipartFile file);

    /**
     * 同步执行文档解析、切分、向量生成和结果替换。
     *
     * <p>耗时的解析与向量生成在事务外完成。只有新结果全部准备好后，才在一个短事务中替换旧分块、
     * 旧向量并更新文档状态。准备阶段或事务阶段失败都会保留原有可用结果，并在文档元数据里记录
     * 最近一次错误，方便观察和重试。</p>
     *
     * @param docId 文档 ID
     * @param requestParam 本次使用的切分策略和参数；传空时沿用文档配置
     * @return 切分后的最新文档信息
     * @throws com.ken.agent.framework.exception.ClientException 文档不存在、正在处理中或策略不支持时抛出
     * @throws com.ken.agent.framework.exception.ServiceException 解析、向量生成或结果保存失败时抛出
     */
    KnowledgeDocumentVO startChunk(String docId, KnowledgeDocumentChunkRequest requestParam);

    /**
     * 删除文档、所属分块和向量，并在数据库删除成功后尽力删除原文件。
     *
     * @param docId 文档 ID
     * @throws com.ken.agent.framework.exception.ClientException 文档不存在或正在切分时抛出
     */
    void deleteDocument(String docId);

    /**
     * 查询文档详情，包含当前处理状态、分块数量和最近一次错误等元数据。
     *
     * @param docId 文档 ID
     * @return 文档详情
     */
    KnowledgeDocumentVO getDocument(String docId);

    /**
     * 分页查询指定知识库中的文档。
     *
     * @param kbId 知识库 ID
     * @param requestParam 分页、关键字和状态筛选参数；传空时使用默认值
     * @return 按最后更新时间倒序排列的文档分页结果
     */
    IPage<KnowledgeDocumentVO> pageDocuments(String kbId, KnowledgeDocumentPageRequest requestParam);
}
