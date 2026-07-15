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
	router := newTestRouter(t, &fakeKnowledgeService{})

	actual := make([]string, 0, len(router.Routes()))
	for _, route := range router.Routes() {
		actual = append(actual, route.Method+" "+route.Path)
	}
	sort.Strings(actual)
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
	service := &fakeKnowledgeService{uploadDocument: domain.Document{
		ID:     "doc-1",
		KbID:   "kb-1",
		Status: domain.StatusPending,
	}}
	router := newTestRouter(t, service)

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

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/knowledge/bases/kb-1/documents/upload",
		&body,
	)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
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
	service := &fakeKnowledgeService{
		processDocument: domain.Document{ID: "doc-1", KbID: "kb-1", Status: domain.StatusCompleted},
		listDocuments:   []domain.Document{{ID: "doc-1", KbID: "kb-1", Status: domain.StatusPending}},
		listTotal:       1,
	}
	router := newTestRouter(t, service)

	chunkRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/knowledge/documents/doc-1/chunk",
		strings.NewReader(`{"minChars":10,"targetChars":20,"maxChars":30}`),
	)
	chunkRequest.Header.Set("Content-Type", "application/json")
	chunkRecorder := httptest.NewRecorder()
	router.ServeHTTP(chunkRecorder, chunkRequest)
	if chunkRecorder.Code != http.StatusOK || service.processID != "doc-1" || service.processOptions.TargetChars != 20 {
		t.Fatalf("chunk result = status:%d id:%q options:%#v", chunkRecorder.Code, service.processID, service.processOptions)
	}

	listRecorder := httptest.NewRecorder()
	router.ServeHTTP(listRecorder, httptest.NewRequest(
		http.MethodGet,
		"/api/knowledge/bases/kb-1/documents?current=2&size=5",
		nil,
	))
	if listRecorder.Code != http.StatusOK || service.listKBID != "kb-1" || service.listPage != 2 || service.listSize != 5 {
		t.Fatalf("list result = status:%d kb:%q page:%d size:%d", listRecorder.Code, service.listKBID, service.listPage, service.listSize)
	}
	var listResult struct {
		Data PageResult `json:"data"`
	}
	if err := json.Unmarshal(listRecorder.Body.Bytes(), &listResult); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if listResult.Data.Total != 1 || listResult.Data.Current != 2 || listResult.Data.Size != 5 {
		t.Fatalf("page response = %#v", listResult.Data)
	}

	deleteRecorder := httptest.NewRecorder()
	router.ServeHTTP(deleteRecorder, httptest.NewRequest(http.MethodDelete, "/api/knowledge/documents/doc-1", nil))
	if deleteRecorder.Code != http.StatusOK || service.deletedID != "doc-1" {
		t.Fatalf("delete result = status:%d id:%q", deleteRecorder.Code, service.deletedID)
	}
}

// TestHandlerMapsNotFoundError 验证服务返回包装后的 ErrNotFound 时仍输出 404 和客户端业务码。
func TestHandlerMapsNotFoundError(t *testing.T) {
	service := &fakeKnowledgeService{getError: domain.ErrNotFound}
	router := newTestRouter(t, service)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/knowledge/documents/missing", nil))

	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), `"code":"A000001"`) {
		t.Fatalf("not-found response = status:%d body:%s", recorder.Code, recorder.Body.String())
	}
}

// newTestRouter 用空输出 slog 和真实中间件构造路由，测试不会向终端打印访问日志。
func newTestRouter(t *testing.T, service KnowledgeService) *gin.Engine {
	t.Helper()
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
	uploadInput     application.UploadInput
	uploadContent   []byte
	uploadDocument  domain.Document
	processID       string
	processOptions  domain.ChunkOptions
	processDocument domain.Document
	getDocument     domain.Document
	getError        error
	listKBID        string
	listPage        int
	listSize        int
	listDocuments   []domain.Document
	listTotal       int64
	deletedID       string
}

func (service *fakeKnowledgeService) UploadDocument(
	_ context.Context,
	input application.UploadInput,
) (domain.Document, error) {
	service.uploadInput = input
	service.uploadContent, _ = io.ReadAll(input.Content)
	return service.uploadDocument, nil
}

func (service *fakeKnowledgeService) ProcessDocument(
	_ context.Context,
	documentID string,
	options domain.ChunkOptions,
) (domain.Document, error) {
	service.processID = documentID
	service.processOptions = options
	return service.processDocument, nil
}

func (service *fakeKnowledgeService) GetDocument(_ context.Context, documentID string) (domain.Document, error) {
	service.processID = documentID
	return service.getDocument, service.getError
}

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

func (service *fakeKnowledgeService) DeleteDocument(_ context.Context, documentID string) error {
	service.deletedID = documentID
	return nil
}
