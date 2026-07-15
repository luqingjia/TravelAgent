package httpadapter

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/luqingjia/TravelAgent/internal/knowledge/application"
	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
	"github.com/luqingjia/TravelAgent/internal/platform/httpserver"
)

// KnowledgeService 是 HTTP Handler 真正调用的应用用例集合。
// 接口定义在使用方 HTTP 适配器，测试可以传 fake，生产则注入 *application.Service。
type KnowledgeService interface {
	UploadDocument(context.Context, application.UploadInput) (domain.Document, error)
	ProcessDocument(context.Context, string, domain.ChunkOptions) (domain.Document, error)
	GetDocument(context.Context, string) (domain.Document, error)
	ListDocuments(context.Context, string, int, int) ([]domain.Document, int64, error)
	DeleteDocument(context.Context, string) error
}

// Handler 保存已经构造好的应用服务和日志器。
// 依赖通过构造函数长期保存，绝不会从 gin.Context 里按字符串查找数据库或 service。
type Handler struct {
	service KnowledgeService
	logger  *slog.Logger
}

// 编译期确认应用服务直接满足 Handler 需要的用例接口。
var _ KnowledgeService = (*application.Service)(nil)

// NewHandler 校验依赖并创建知识接口处理器。
func NewHandler(service KnowledgeService, logger *slog.Logger) (*Handler, error) {
	if isNilService(service) {
		return nil, fmt.Errorf("knowledge application service is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("knowledge HTTP logger is required")
	}
	return &Handler{service: service, logger: logger}, nil
}

// upload 把 multipart 文件和表单字段转换成框架无关的上传输入。
func (handler *Handler) upload(context *gin.Context) {
	file, err := context.FormFile("file")
	if err != nil {
		handler.writeError(context, fmt.Errorf("%w: multipart file is required", domain.ErrInvalidArgument))
		return
	}
	reader, err := file.Open()
	if err != nil {
		handler.writeError(context, fmt.Errorf("open uploaded file: %w", err))
		return
	}
	defer reader.Close()

	chunkConfig, err := optionalJSONFormMap(context, "chunkConfig")
	if err != nil {
		handler.writeError(context, err)
		return
	}
	metadata, err := optionalJSONFormMap(context, "metadata")
	if err != nil {
		handler.writeError(context, err)
		return
	}

	// multipart.FileHeader 只存在于本函数；应用层接收普通 reader、大小和字符串字段。
	document, err := handler.service.UploadDocument(context.Request.Context(), application.UploadInput{
		KnowledgeBaseID: context.Param("kbID"),
		FileName:        file.Filename,
		Title:           context.PostForm("title"),
		ContentType:     file.Header.Get("Content-Type"),
		Language:        context.PostForm("language"),
		ChunkStrategy:   context.PostForm("chunkStrategy"),
		ChunkConfig:     chunkConfig,
		Metadata:        metadata,
		Content:         reader,
		Size:            file.Size,
	})
	if err != nil {
		handler.writeError(context, err)
		return
	}

	context.JSON(http.StatusOK, Success(documentResponseFromDomain(document)))
}

// processDocument 解析可选 JSON 分块参数并显式触发文档处理。
func (handler *Handler) processDocument(context *gin.Context) {
	request := chunkRequest{}
	if context.Request.Body != nil && context.Request.ContentLength != 0 {
		if err := context.ShouldBindJSON(&request); err != nil {
			handler.writeError(context, fmt.Errorf("%w: invalid chunk options", domain.ErrInvalidArgument))
			return
		}
	}

	document, err := handler.service.ProcessDocument(
		context.Request.Context(),
		context.Param("docID"),
		request.toDomain(),
	)
	if err != nil {
		handler.writeError(context, err)
		return
	}
	context.JSON(http.StatusOK, Success(documentResponseFromDomain(document)))
}

// getDocument 查询文档详情；状态接口复用同一处理器以保持旧行为。
func (handler *Handler) getDocument(context *gin.Context) {
	document, err := handler.service.GetDocument(context.Request.Context(), context.Param("docID"))
	if err != nil {
		handler.writeError(context, err)
		return
	}
	context.JSON(http.StatusOK, Success(documentResponseFromDomain(document)))
}

// listDocuments 读取 current/size 查询参数，并返回兼容分页结构。
func (handler *Handler) listDocuments(context *gin.Context) {
	page := positiveIntQuery(context, "current", 1)
	size := positiveIntQuery(context, "size", 20)
	documents, total, err := handler.service.ListDocuments(
		context.Request.Context(),
		context.Param("kbID"),
		page,
		size,
	)
	if err != nil {
		handler.writeError(context, err)
		return
	}

	context.JSON(http.StatusOK, Success(PageResult{
		Records: documentResponsesFromDomain(documents),
		Total:   total,
		Current: page,
		Size:    size,
	}))
}

// deleteDocument 执行删除用例；成功时 data 保持 null，与原接口语义一致。
func (handler *Handler) deleteDocument(context *gin.Context) {
	if err := handler.service.DeleteDocument(context.Request.Context(), context.Param("docID")); err != nil {
		handler.writeError(context, err)
		return
	}
	context.JSON(http.StatusOK, Success(nil))
}

// writeError 统一输出错误响应，并为服务端故障记录结构化上下文。
func (handler *Handler) writeError(context *gin.Context, err error) {
	status, result := errorResponse(err)
	if status >= http.StatusInternalServerError {
		handler.logger.ErrorContext(
			context.Request.Context(),
			"knowledge HTTP request failed",
			"request_id", httpserver.RequestID(context),
			"method", context.Request.Method,
			"path", context.Request.URL.Path,
			"error", err,
		)
	}
	context.AbortWithStatusJSON(status, result)
}

// positiveIntQuery 解析正整数查询参数；缺失、格式错误或非正数时使用兼容默认值。
func positiveIntQuery(context *gin.Context, name string, fallback int) int {
	value := context.Query(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

// isNilService 同时识别普通 nil 和接口内部保存的 nil 指针，避免构造成功后第一次调用才 panic。
func isNilService(service KnowledgeService) bool {
	if service == nil {
		return true
	}
	value := reflect.ValueOf(service)
	return value.Kind() == reflect.Pointer && value.IsNil()
}
