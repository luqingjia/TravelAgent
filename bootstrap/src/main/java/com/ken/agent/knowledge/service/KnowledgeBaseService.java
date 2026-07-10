package com.ken.agent.knowledge.service;

import com.baomidou.mybatisplus.core.metadata.IPage;
import com.baomidou.mybatisplus.extension.service.IService;
import com.ken.agent.knowledge.controller.request.KnowledgeBaseCreateRequest;
import com.ken.agent.knowledge.controller.request.KnowledgeBasePageRequest;
import com.ken.agent.knowledge.controller.request.KnowledgeBaseUpdateRequest;
import com.ken.agent.knowledge.dao.entity.KnowledgeBaseEntity;
import com.ken.agent.knowledge.dao.vo.KnowledgeBaseVO;

/**
 * 知识库业务接口。
 *
 * <p>这里管理的是知识库这个“文件夹”本身，不负责上传文件、切分文本或写入向量。</p>
 */
public interface KnowledgeBaseService extends IService<KnowledgeBaseEntity> {

    /**
     * 新建一个知识库。
     *
     * @param requestParam 知识库名称、说明、可见范围等创建参数
     * @return 创建完成后的知识库信息
     * @throws com.ken.agent.framework.exception.ClientException 名称为空时抛出
     * @throws com.ken.agent.framework.exception.ServiceException 名称已经存在或保存失败时抛出
     */
    KnowledgeBaseVO createKnowledgeBase(KnowledgeBaseCreateRequest requestParam);

    /**
     * 修改指定知识库的基本资料；没有传入的字段保持原值。
     *
     * @param kbId 知识库 ID
     * @param requestParam 本次需要修改的字段
     * @return 修改后的知识库信息
     * @throws com.ken.agent.framework.exception.ClientException 知识库不存在时抛出
     */
    KnowledgeBaseVO updateKnowledgeBase(String kbId, KnowledgeBaseUpdateRequest requestParam);

    /**
     * 删除知识库。
     *
     * <p>知识库下还有文档时不允许删除，防止留下找不到归属的文档。</p>
     *
     * @param kbId 知识库 ID
     * @throws com.ken.agent.framework.exception.ClientException 知识库不存在或仍包含文档时抛出
     */
    void deleteKnowledgeBase(String kbId);

    /**
     * 查询一个知识库的详情。
     *
     * @param kbId 知识库 ID
     * @return 知识库详情
     * @throws com.ken.agent.framework.exception.ClientException 知识库不存在时抛出
     */
    KnowledgeBaseVO getKnowledgeBase(String kbId);

    /**
     * 按名称关键字、类型和状态分页查询知识库。
     *
     * @param requestParam 分页和筛选参数；传空时使用默认分页参数
     * @return 按最后更新时间倒序排列的分页结果
     */
    IPage<KnowledgeBaseVO> pageKnowledgeBases(KnowledgeBasePageRequest requestParam);
}
