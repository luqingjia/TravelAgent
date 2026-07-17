package httpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/luqingjia/TravelAgent/internal/knowledge/application"
	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
	"github.com/luqingjia/TravelAgent/internal/platform/httpserver"
)

// TestRouterRegistersHealthAndKnowledgeRoutes 验证重构后仍精确注册健康检查和六条知识接口。
func TestRouterRegistersHealthAndKnowledgeRoutes(t *testing.T) {
	// 使用真实 Router 和 fake 服务，路由表可以直接反映最终暴露给客户端的接口。
	router := newTestRouter(t, &fakeKnowledgeService{})

	// Gin 返回的路由顺序不是本测试关心的内容，所以先统一转成“方法 + 路径”并排序。
	actual := make([]string, 0, len(router.Routes()))
	for _, route := range router.Routes() {
		actual = append(actual, route.Method+" "+route.Path)
	}
	sort.Strings(actual)
	// want 明确列出兼容合同，接口缺失、路径漂移或方法改变都会触发失败。
	want := []string{
		"DELETE /api/knowledge/documents/:docID",
		"GET /api/knowledge/bases/:kbID/documents",
		"GET /api/knowledge/documents/:docID",
		"GET /api/knowledge/documents/:docID/status",
		"GET /health",
		"POST /api/knowledge/bases/:kbID/documents/upload",
		"POST /api/knowledge/documents/:docID/chunk",
	}
	sort.Strings(want)
	if strings.Join(actual, "\n") != strings.Join(want, "\n") {
		t.Fatalf("routes =\n%s\nwant =\n%s", strings.Join(actual, "\n"), strings.Join(want, "\n"))
	}
}

// TestUploadHandlerBuildsFrameworkFreeInput 验证 Gin multipart 数据会转换成 application.UploadInput，
// 应用服务拿到普通 reader 和字段，不需要认识 gin.Context 或 multipart.FileHeader。
func TestUploadHandlerBuildsFrameworkFreeInput(t *testing.T) {
	// fake 服务预设返回 pending 文档，同时记录 Handler 传入的应用层对象和文件内容。
	service := &fakeKnowledgeService{uploadDocument: domain.Document{
		ID:     "doc-1",
		KbID:   "kb-1",
		Status: domain.StatusPending,
	}}
	router := newTestRouter(t, service)

	// 按浏览器真实上传格式构造 multipart 请求，覆盖文件和三个普通表单字段。
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fileWriter, err := writer.CreateFormFile("file", "guide.md")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	_, _ = fileWriter.Write([]byte("hello DDD"))
	_ = writer.WriteField("title", "Go 指南")
	_ = writer.WriteField("language", "zh")
	_ = writer.WriteField("chunkStrategy", "structure_aware")
	if err := writer.Close(); err != nil {
		t.Fatalf("multipart Close() error = %v", err)
	}

	// 请求路径中的 kb-1 应被取出并写入 UploadInput.KnowledgeBaseID。
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/knowledge/bases/kb-1/documents/upload",
		&body,
	)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	// recorder 保存 Handler 最终写出的状态码、响应头和 JSON 响应体。
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	// HTTP 层成功接收并委托后必须保持原接口的 200 响应。
	if recorder.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	// fake 中记录的值证明 Handler 已完成边界转换，没有把 Gin 对象传进应用层。
	if service.uploadInput.KnowledgeBaseID != "kb-1" ||
		service.uploadInput.FileName != "guide.md" ||
		service.uploadInput.Title != "Go 指南" ||
		string(service.uploadContent) != "hello DDD" {
		t.Fatalf("UploadInput = %#v, content = %q", service.uploadInput, service.uploadContent)
	}

	// 响应必须使用旧 camelCase DTO，不能直接暴露领域结构体的 Go 字段名。
	if !strings.Contains(recorder.Body.String(), `"kbId":"kb-1"`) || strings.Contains(recorder.Body.String(), `"KbID"`) {
		t.Fatalf("upload response DTO = %s", recorder.Body.String())
	}
}

// TestChunkListAndDeleteHandlersDelegateParameters 验证 JSON 分块参数、分页查询和删除都只做边界转换后委托服务。
func TestChunkListAndDeleteHandlersDelegateParameters(t *testing.T) {
	// 一次准备三种用例的返回值，后续分别检查分块、列表和删除参数有没有正确下传。
	service := &fakeKnowledgeService{
		processDocument: domain.Document{ID: "doc-1", KbID: "kb-1", Status: domain.StatusCompleted},
		listDocuments:   []domain.Document{{ID: "doc-1", KbID: "kb-1", Status: domain.StatusPending}},
		listTotal:       1,
	}
	router := newTestRouter(t, service)

	// 第一段：JSON 中的三个分块阈值应转换为领域 ChunkOptions。
	chunkRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/knowledge/documents/doc-1/chunk",
		strings.NewReader(`{"minChars":10,"targetChars":20,"maxChars":30}`),
	)
	chunkRequest.Header.Set("Content-Type", "application/json")
	chunkRecorder := httptest.NewRecorder()
	router.ServeHTTP(chunkRecorder, chunkRequest)
	// 状态码、路径参数和关键阈值一起断言，任意边界转换丢失都会被发现。
	if chunkRecorder.Code != http.StatusOK || service.processID != "doc-1" || service.processOptions.TargetChars != 20 {
		t.Fatalf("chunk result = status:%d id:%q options:%#v", chunkRecorder.Code, service.processID, service.processOptions)
	}

	// 第二段：查询字符串 current/size 应原样交给列表用例。
	listRecorder := httptest.NewRecorder()
	router.ServeHTTP(listRecorder, httptest.NewRequest(
		http.MethodGet,
		"/api/knowledge/bases/kb-1/documents?current=2&size=5",
		nil,
	))
	if listRecorder.Code != http.StatusOK || service.listKBID != "kb-1" || service.listPage != 2 || service.listSize != 5 {
		t.Fatalf("list result = status:%d kb:%q page:%d size:%d", listRecorder.Code, service.listKBID, service.listPage, service.listSize)
	}
	// 反序列化统一响应外壳，检查分页元数据没有在 DTO 转换中丢失。
	var listResult struct {
		Data PageResult `json:"data"`
	}
	if err := json.Unmarshal(listRecorder.Body.Bytes(), &listResult); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if listResult.Data.Total != 1 || listResult.Data.Current != 2 || listResult.Data.Size != 5 {
		t.Fatalf("page response = %#v", listResult.Data)
	}

	// 第三段：删除只需要把路径中的文档编号交给服务，并返回成功外壳。
	deleteRecorder := httptest.NewRecorder()
	router.ServeHTTP(deleteRecorder, httptest.NewRequest(http.MethodDelete, "/api/knowledge/documents/doc-1", nil))
	if deleteRecorder.Code != http.StatusOK || service.deletedID != "doc-1" {
		t.Fatalf("delete result = status:%d id:%q", deleteRecorder.Code, service.deletedID)
	}
}

// TestHandlerMapsNotFoundError 验证服务返回包装后的 ErrNotFound 时仍输出 404 和客户端业务码。
func TestHandlerMapsNotFoundError(t *testing.T) {
	// fake 返回领域层的“不存在”错误，Handler 负责把它翻译成稳定 HTTP 合同。
	service := &fakeKnowledgeService{getError: domain.ErrNotFound}
	router := newTestRouter(t, service)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/knowledge/documents/missing", nil))

	// 同时检查 404 和客户端业务码，避免只改了其中一层导致前端判断失效。
	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), `"code":"A000001"`) {
		t.Fatalf("not-found response = status:%d body:%s", recorder.Code, recorder.Body.String())
	}
}

// newTestRouter 用空输出 slog 和真实中间件构造路由，测试不会向终端打印访问日志。
func newTestRouter(t *testing.T, service KnowledgeService) *gin.Engine {
	// 标记帮助函数，让构造错误定位到调用它的具体测试。
	t.Helper()
	// 日志丢弃，但 Handler 和中间件仍使用生产构造器，避免测试绕过真实校验。
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler, err := NewHandler(service, logger)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	middleware, err := httpserver.NewMiddleware(logger)
	if err != nil {
		t.Fatalf("NewMiddleware() error = %v", err)
	}
	return NewRouter(handler, middleware)
}

// fakeKnowledgeService 记录每个 Handler 传入的参数，并返回测试预设结果。
type fakeKnowledgeService struct {
	// uploadInput 和 uploadContent 保存上传边界转换后的普通 Go 数据。
	uploadInput   application.UploadInput
	uploadContent []byte
	// uploadDocument 是上传用例返回给 Handler 的预设领域结果。
	uploadDocument domain.Document
	// processID、processOptions 记录分块接口下传的路径参数和 JSON 配置。
	processID      string
	processOptions domain.ChunkOptions
	// processDocument 是分块接口成功后返回的预设文档。
	processDocument domain.Document
	// getDocument 和 getError 分别控制单文档查询的成功值与失败值。
	getDocument domain.Document
	getError    error
	// listKBID、listPage、listSize 记录列表接口收到的查询条件。
	listKBID string
	listPage int
	listSize int
	// listDocuments 和 listTotal 是分页接口返回的预设数据。
	listDocuments []domain.Document
	listTotal     int64
	// deletedID 记录删除接口最终传入的文档编号。
	deletedID string
}

// UploadDocument 读取上传内容并保存全部输入，随后返回测试预设文档。
func (service *fakeKnowledgeService) UploadDocument(
	_ context.Context,
	input application.UploadInput,
) (domain.Document, error) {
	// 保存结构化字段，供测试确认路径和表单值已经脱离 Gin。
	service.uploadInput = input
	// 立即读完 Reader，模拟应用服务真正消费文件内容。
	service.uploadContent, _ = io.ReadAll(input.Content)
	return service.uploadDocument, nil
}

// ProcessDocument 记录路径编号和分块配置，随后返回测试预设结果。
func (service *fakeKnowledgeService) ProcessDocument(
	_ context.Context,
	documentID string,
	options domain.ChunkOptions,
) (domain.Document, error) {
	service.processID = documentID
	service.processOptions = options
	return service.processDocument, nil
}

// GetDocument 记录查询编号，并按场景返回预设文档或错误。
func (service *fakeKnowledgeService) GetDocument(_ context.Context, documentID string) (domain.Document, error) {
	service.processID = documentID
	return service.getDocument, service.getError
}

// ListDocuments 保存分页参数，并返回预设列表和总数。
func (service *fakeKnowledgeService) ListDocuments(
	_ context.Context,
	knowledgeBaseID string,
	page int,
	size int,
) ([]domain.Document, int64, error) {
	service.listKBID = knowledgeBaseID
	service.listPage = page
	service.listSize = size
	return service.listDocuments, service.listTotal, nil
}

// DeleteDocument 记录待删除编号并模拟成功。
func (service *fakeKnowledgeService) DeleteDocument(_ context.Context, documentID string) error {
	service.deletedID = documentID
	return nil
}
