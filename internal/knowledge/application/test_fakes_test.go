package application

import (
	"context"

	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// fakeRepository 是应用层测试使用的内存记录器。
// 每个错误开关模拟一个真实基础设施故障，调用记录则用于断言业务流程有没有越过不该越过的边界。
type fakeRepository struct {
	// 以下字段控制知识库存在性和内容哈希去重查询的返回结果。
	knowledgeBaseExists    bool
	knowledgeBaseExistsErr error
	duplicate              bool
	duplicateErr           error
	// createErr 控制新建文档写入是否失败。
	createErr error
	// getDocument、getErr 控制单文档查询结果。
	getDocument domain.Document
	getErr      error
	// listDocuments、listTotal、listErr 控制分页查询结果。
	listDocuments []domain.Document
	listTotal     int64
	listErr       error
	// deleteErr 控制删除文档时的基础设施故障。
	deleteErr error
	// processing 相关字段模拟原子抢占返回的文档、是否抢到以及查询错误。
	processingDocument domain.Document
	processingAcquired bool
	processingErr      error
	// replaceErr 和 markFailedErr 分别控制成功替换事务与失败回写故障。
	replaceErr    error
	markFailedErr error

	// createdDocuments 保存每次创建的新文档聚合。
	createdDocuments []domain.Document
	// replacedDocuments、replacedChunks、replacedVectors 保存每次原子替换的完整参数。
	replacedDocuments []domain.Document
	replacedChunks    [][]domain.Chunk
	replacedVectors   [][][]float32
	// failedDocuments 保存应用层要求仓储回写的 failed 聚合。
	failedDocuments []domain.Document
	// 以下切片分别记录查询、分页、删除和抢占时实际传入的参数。
	getDocIDs        []string
	listRequests     []listRequest
	deleteDocIDs     []string
	processingDocIDs []string
}

// listRequest 保存一次分页查询参数，帮助测试确认默认页码和页大小确实传到了仓储边界。
type listRequest struct {
	// kbID 是要查询的知识库编号。
	kbID string
	// page 和 size 是应用层校正后的页码与每页数量。
	page int
	size int
}

// KnowledgeBaseExists 返回测试预设的存在性和错误，不访问真实数据库。
func (f *fakeRepository) KnowledgeBaseExists(_ context.Context, _ string) (bool, error) {
	return f.knowledgeBaseExists, f.knowledgeBaseExistsErr
}

// ActiveDocumentHashExists 返回测试预设的重复判断和错误。
func (f *fakeRepository) ActiveDocumentHashExists(_ context.Context, _, _ string) (bool, error) {
	return f.duplicate, f.duplicateErr
}

// CreateDocument 先记录完整聚合，再返回注入的写入错误。
func (f *fakeRepository) CreateDocument(_ context.Context, document domain.Document) error {
	f.createdDocuments = append(f.createdDocuments, document)
	return f.createErr
}

// GetDocument 记录文档编号并返回预设查询结果。
func (f *fakeRepository) GetDocument(_ context.Context, docID string) (domain.Document, error) {
	f.getDocIDs = append(f.getDocIDs, docID)
	return f.getDocument, f.getErr
}

// ListDocuments 记录校正后的分页条件并返回预设页数据。
func (f *fakeRepository) ListDocuments(_ context.Context, kbID string, page, size int) ([]domain.Document, int64, error) {
	f.listRequests = append(f.listRequests, listRequest{kbID: kbID, page: page, size: size})
	return f.listDocuments, f.listTotal, f.listErr
}

// DeleteDocument 记录文档编号并返回预设删除错误。
func (f *fakeRepository) DeleteDocument(_ context.Context, docID string) error {
	f.deleteDocIDs = append(f.deleteDocIDs, docID)
	return f.deleteErr
}

// TryMarkProcessing 记录抢占目标并返回预设的原子抢占结果。
func (f *fakeRepository) TryMarkProcessing(_ context.Context, docID string) (domain.Document, bool, error) {
	f.processingDocIDs = append(f.processingDocIDs, docID)
	return f.processingDocument, f.processingAcquired, f.processingErr
}

// ReplaceDocumentChunks 保存事务的三组输入，让测试检查文档、分块和向量是否一一对应。
func (f *fakeRepository) ReplaceDocumentChunks(_ context.Context, document domain.Document, chunks []domain.Chunk, vectors [][]float32) error {
	f.replacedDocuments = append(f.replacedDocuments, document)
	f.replacedChunks = append(f.replacedChunks, chunks)
	f.replacedVectors = append(f.replacedVectors, vectors)
	return f.replaceErr
}

// MarkFailed 记录待回写的失败聚合并返回注入错误。
func (f *fakeRepository) MarkFailed(_ context.Context, document domain.Document) error {
	f.failedDocuments = append(f.failedDocuments, document)
	return f.markFailedErr
}

// fakeStorage 记录对象存储的 Put/Get/Delete 调用，并允许测试注入对应故障。
type fakeStorage struct {
	// putResult 和 putErr 控制写入对象后的返回值与故障。
	putResult StoredObject
	putErr    error
	// getResult 和 getErr 控制读取原文件的内容与故障。
	getResult []byte
	getErr    error
	// deleteErr 控制补偿删除是否失败。
	deleteErr error

	// 三组调用记录用于确认流程有没有越过边界，以及使用了哪个 URI。
	putInputs  []StoredObjectInput
	getURIs    []string
	deleteURIs []string
}

// Put 记录待写入对象并返回预设结果。
func (f *fakeStorage) Put(_ context.Context, input StoredObjectInput) (StoredObject, error) {
	f.putInputs = append(f.putInputs, input)
	return f.putResult, f.putErr
}

// Get 记录读取 URI 并返回预设字节。
func (f *fakeStorage) Get(_ context.Context, uri string) ([]byte, error) {
	f.getURIs = append(f.getURIs, uri)
	return f.getResult, f.getErr
}

// Delete 记录补偿删除 URI 并返回预设错误。
func (f *fakeStorage) Delete(_ context.Context, uri string) error {
	f.deleteURIs = append(f.deleteURIs, uri)
	return f.deleteErr
}

// fakeEmbedder 模拟批量向量模型，并记录输入文本，便于断言失败时是否错误调用了模型。
type fakeEmbedder struct {
	// vectors 是按调用顺序消费的返回队列，每一项代表一次批量 Embedding 结果。
	vectors [][][]float32
	// err 是模型调用要返回的预设错误。
	err error
	// texts 保存每次送给模型的文本副本，防止调用方后续修改影响断言。
	texts [][]string
}

// EmbedTexts 记录输入并按队列顺序返回向量，模拟可重复调用的批量模型客户端。
func (f *fakeEmbedder) EmbedTexts(_ context.Context, texts []string) ([][]float32, error) {
	// 复制切片后再保存，避免与调用方共享底层数组导致记录被改写。
	f.texts = append(f.texts, append([]string(nil), texts...))
	// 没有预设向量时只返回注入错误，适合验证调用是否发生。
	if len(f.vectors) == 0 {
		return nil, f.err
	}
	// 取出队首结果并推进队列，让多次调用可以得到不同批次。
	result := f.vectors[0]
	f.vectors = f.vectors[1:]
	return result, f.err
}
