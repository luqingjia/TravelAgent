package com.ken.agent.knowledge.service;

import com.baomidou.mybatisplus.core.metadata.IPage;
import com.baomidou.mybatisplus.extension.service.IService;
import com.ken.agent.knowledge.controller.request.KnowledgeChunkCreateRequest;
import com.ken.agent.knowledge.controller.request.KnowledgeChunkPageRequest;
import com.ken.agent.knowledge.controller.request.KnowledgeChunkUpdateRequest;
import com.ken.agent.knowledge.dao.entity.KnowledgeChunkEntity;
import com.ken.agent.knowledge.dao.vo.KnowledgeChunkVO;

import java.util.List;

/**
 * 文档分块业务接口。
 *
 * <p>一个分块就是文档中可单独检索的一小段文字。手工增删改分块时，服务会同步维护文档的
 * 分块数量和对应向量，避免普通数据与向量数据不一致。</p>
 */
public interface KnowledgeChunkService extends IService<KnowledgeChunkEntity> {

    /**
     * 分页查看某个文档的分块。
     *
     * @param docId 文档 ID
     * @param requestParam 分页和启用状态筛选参数；传空时使用默认值
     * @return 按分块顺序排列的分页结果
     */
    IPage<KnowledgeChunkVO> pageChunks(String docId, KnowledgeChunkPageRequest requestParam);

    /**
     * 给文档手工增加一个分块。
     *
     * <p>如果文档已经切分完成，会同时生成并保存这个新分块的向量；任一步失败都会回滚数据库改动。</p>
     *
     * @param docId 文档 ID
     * @param requestParam 分块内容、顺序和附加信息
     * @return 新建的分块
     */
    KnowledgeChunkVO createChunk(String docId, KnowledgeChunkCreateRequest requestParam);

    /**
     * 修改一个已有分块。
     *
     * <p>已完成文档中的启用分块会重新生成向量；失败时文字和向量一起回滚。</p>
     *
     * @param docId 文档 ID
     * @param chunkId 分块 ID
     * @param requestParam 新的分块内容和附加信息
     * @return 修改后的分块
     */
    KnowledgeChunkVO updateChunk(String docId, String chunkId, KnowledgeChunkUpdateRequest requestParam);

    /**
     * 启用或停用一个分块。
     *
     * <p>停用会删除它的向量；重新启用已完成文档中的分块会重新生成向量。</p>
     *
     * @param docId 文档 ID
     * @param chunkId 分块 ID
     * @param enabled {@code true} 表示启用，{@code false} 表示停用
     */
    void enableChunk(String docId, String chunkId, boolean enabled);

    /**
     * 逻辑删除一个分块，并同步删除向量、减少文档分块数量。
     *
     * @param docId 文档 ID
     * @param chunkId 分块 ID
     */
    void deleteChunk(String docId, String chunkId);

    /**
     * 逻辑删除一个文档下的全部分块。
     *
     * <p>用于正常删除文档，保留逻辑删除痕迹。</p>
     *
     * @param docId 文档 ID；为空时直接返回
     */
    void deleteByDocumentId(String docId);

    /**
     * 物理删除一个文档下的全部分块。
     *
     * <p>只用于“重新切分后原子替换”场景。旧行必须真正移除，否则相同分块 ID 或顺序可能与新结果冲突。</p>
     *
     * @param docId 文档 ID；为空时直接返回
     */
    void deletePhysicallyByDocumentId(String docId);

    /**
     * 查询一个文档的全部有效分块。
     *
     * @param docId 文档 ID
     * @return 按分块顺序排列的列表
     */
    List<KnowledgeChunkVO> listByDocumentId(String docId);
}
