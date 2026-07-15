package application

import (
	"context"

	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// fakeRepository 是应用层测试使用的内存记录器。
// 每个错误开关模拟一个真实基础设施故障，调用记录则用于断言业务流程有没有越过不该越过的边界。
type fakeRepository struct {
	knowledgeBaseExists    bool
	knowledgeBaseExistsErr error
	duplicate              bool
	duplicateErr           error
	createErr              error
	getDocument            domain.Document
	getErr                 error
	listDocuments          []domain.Document
	listTotal              int64
	listErr                error
	deleteErr              error
	processingDocument     domain.Document
	processingAcquired     bool
	processingErr          error
	replaceErr             error
	markFailedErr          error

	createdDocuments  []domain.Document
	replacedDocuments []domain.Document
	replacedChunks    [][]domain.Chunk
	replacedVectors   [][][]float32
	failedDocuments   []domain.Document
	getDocIDs         []string
	listRequests      []listRequest
	deleteDocIDs      []string
	processingDocIDs  []string
}

// listRequest 保存一次分页查询参数，帮助测试确认默认页码和页大小确实传到了仓储边界。
type listRequest struct {
	kbID string
	page int
	size int
}

func (f *fakeRepository) KnowledgeBaseExists(_ context.Context, _ string) (bool, error) {
	return f.knowledgeBaseExists, f.knowledgeBaseExistsErr
}

func (f *fakeRepository) ActiveDocumentHashExists(_ context.Context, _, _ string) (bool, error) {
	return f.duplicate, f.duplicateErr
}

func (f *fakeRepository) CreateDocument(_ context.Context, document domain.Document) error {
	f.createdDocuments = append(f.createdDocuments, document)
	return f.createErr
}

func (f *fakeRepository) GetDocument(_ context.Context, docID string) (domain.Document, error) {
	f.getDocIDs = append(f.getDocIDs, docID)
	return f.getDocument, f.getErr
}

func (f *fakeRepository) ListDocuments(_ context.Context, kbID string, page, size int) ([]domain.Document, int64, error) {
	f.listRequests = append(f.listRequests, listRequest{kbID: kbID, page: page, size: size})
	return f.listDocuments, f.listTotal, f.listErr
}

func (f *fakeRepository) DeleteDocument(_ context.Context, docID string) error {
	f.deleteDocIDs = append(f.deleteDocIDs, docID)
	return f.deleteErr
}

func (f *fakeRepository) TryMarkProcessing(_ context.Context, docID string) (domain.Document, bool, error) {
	f.processingDocIDs = append(f.processingDocIDs, docID)
	return f.processingDocument, f.processingAcquired, f.processingErr
}

func (f *fakeRepository) ReplaceDocumentChunks(_ context.Context, document domain.Document, chunks []domain.Chunk, vectors [][]float32) error {
	f.replacedDocuments = append(f.replacedDocuments, document)
	f.replacedChunks = append(f.replacedChunks, chunks)
	f.replacedVectors = append(f.replacedVectors, vectors)
	return f.replaceErr
}

func (f *fakeRepository) MarkFailed(_ context.Context, document domain.Document) error {
	f.failedDocuments = append(f.failedDocuments, document)
	return f.markFailedErr
}

// fakeStorage 记录对象存储的 Put/Get/Delete 调用，并允许测试注入对应故障。
type fakeStorage struct {
	putResult StoredObject
	putErr    error
	getResult []byte
	getErr    error
	deleteErr error

	putInputs  []StoredObjectInput
	getURIs    []string
	deleteURIs []string
}

func (f *fakeStorage) Put(_ context.Context, input StoredObjectInput) (StoredObject, error) {
	f.putInputs = append(f.putInputs, input)
	return f.putResult, f.putErr
}

func (f *fakeStorage) Get(_ context.Context, uri string) ([]byte, error) {
	f.getURIs = append(f.getURIs, uri)
	return f.getResult, f.getErr
}

func (f *fakeStorage) Delete(_ context.Context, uri string) error {
	f.deleteURIs = append(f.deleteURIs, uri)
	return f.deleteErr
}

// fakeEmbedder 模拟批量向量模型，并记录输入文本，便于断言失败时是否错误调用了模型。
type fakeEmbedder struct {
	vectors [][][]float32
	err     error
	texts   [][]string
}

func (f *fakeEmbedder) EmbedTexts(_ context.Context, texts []string) ([][]float32, error) {
	f.texts = append(f.texts, append([]string(nil), texts...))
	if len(f.vectors) == 0 {
		return nil, f.err
	}
	result := f.vectors[0]
	f.vectors = f.vectors[1:]
	return result, f.err
}
