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

// 本文件只负责 HTTP 输入输出编排：从 Gin 取参数、调用应用用例、再写回统一响应。
// 数据库 SQL、对象存储选择和文档状态规则都不允许在 Handler 中实现。
// KnowledgeService 是 HTTP Handler 真正调用的应用用例集合。
// 接口定义在使用方 HTTP 适配器，测试可以传 fake，生产则注入 *application.Service。
type KnowledgeService interface {
	// UploadDocument 接收已经脱离 Gin 的上传输入并登记 pending 文档。
	UploadDocument(context.Context, application.UploadInput) (domain.Document, error)
	// ProcessDocument 显式触发文档解析、分块和向量化。
	ProcessDocument(context.Context, string, domain.ChunkOptions) (domain.Document, error)
	// GetDocument 查询文档详情或状态。
	GetDocument(context.Context, string) (domain.Document, error)
	// ListDocuments 按知识库和分页参数返回文档列表。
	ListDocuments(context.Context, string, int, int) ([]domain.Document, int64, error)
	// DeleteDocument 删除指定文档。
	DeleteDocument(context.Context, string) error
}

// Handler 保存已经构造好的应用服务和日志器。
// 依赖通过构造函数长期保存，绝不会从 gin.Context 里按字符串查找数据库或 service。
type Handler struct {
	// service 是 Handler 唯一允许调用的业务入口。
	service KnowledgeService
	// logger 只记录服务端故障，不向客户端泄漏技术细节。
	logger *slog.Logger
}

// 编译期确认应用服务直接满足 Handler 需要的用例接口。
var _ KnowledgeService = (*application.Service)(nil)

// NewHandler 校验依赖并创建知识接口处理器。
func NewHandler(service KnowledgeService, logger *slog.Logger) (*Handler, error) {
	// 同时识别普通 nil 和接口中装着的 nil 指针，避免首次请求才 panic。
	if isNilService(service) {
		return nil, fmt.Errorf("knowledge application service is required")
	}
	// 没有日志器时无法安全记录 5xx 和 panic 上下文，因此拒绝构造。
	if logger == nil {
		return nil, fmt.Errorf("knowledge HTTP logger is required")
	}
	// 所有依赖合法后保存为长期字段，后续不再从 gin.Context 做服务定位。
	return &Handler{service: service, logger: logger}, nil
}

// upload 把 multipart 文件和表单字段转换成框架无关的上传输入。
func (handler *Handler) upload(context *gin.Context) {
	// FormFile 同时解析 multipart 并取得名为 file 的文件头。
	file, err := context.FormFile("file")
	// 缺少文件或 multipart 解析失败统一归类为参数错误。
	if err != nil {
		handler.writeError(context, fmt.Errorf("%w: multipart file is required", domain.ErrInvalidArgument))
		return
	}
	// 打开文件头对应的流，应用层会在限制长度内读取它。
	reader, err := file.Open()
	// 临时文件或内存缓冲无法打开时保留技术错误链并返回。
	if err != nil {
		handler.writeError(context, fmt.Errorf("open uploaded file: %w", err))
		return
	}
	// Handler 返回前关闭 multipart reader，避免临时文件描述符泄漏。
	defer reader.Close()

	// chunkConfig 是可选 JSON 对象字段，解析失败时不调用应用服务。
	chunkConfig, err := optionalJSONFormMap(context, "chunkConfig")
	if err != nil {
		handler.writeError(context, err)
		return
	}
	// metadata 使用同一边界解析规则，确保进入应用层时已经是普通 map。
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
	// 应用层错误交给统一映射，不在每个 Handler 中重复判断 errors.Is。
	if err != nil {
		handler.writeError(context, err)
		return
	}

	// 成功文档先转换成 HTTP DTO，再放入兼容 Result 外壳返回。
	context.JSON(http.StatusOK, Success(documentResponseFromDomain(document)))
}

// processDocument 解析可选 JSON 分块参数并显式触发文档处理。
func (handler *Handler) processDocument(context *gin.Context) {
	// 空结构体代表调用方不提供配置，领域 Normalize 会补默认阈值。
	request := chunkRequest{}
	// 没有请求体时允许使用默认值；只有确实存在内容时才执行 JSON 绑定。
	if context.Request.Body != nil && context.Request.ContentLength != 0 {
		// JSON 语法或字段类型错误统一映射为参数错误，不把 Gin 错误文本暴露给客户端。
		if err := context.ShouldBindJSON(&request); err != nil {
			handler.writeError(context, fmt.Errorf("%w: invalid chunk options", domain.ErrInvalidArgument))
			return
		}
	}

	// 传递标准库请求 Context，让客户端取消、超时和链路信息继续向下游传播。
	document, err := handler.service.ProcessDocument(
		context.Request.Context(),
		context.Param("docID"),
		request.toDomain(),
	)
	// 处理冲突、参数错误和基础设施故障统一走 writeError。
	if err != nil {
		handler.writeError(context, err)
		return
	}
	// 只有完整处理成功才返回 completed 文档 DTO。
	context.JSON(http.StatusOK, Success(documentResponseFromDomain(document)))
}

// getDocument 查询文档详情；状态接口复用同一处理器以保持旧行为。
func (handler *Handler) getDocument(context *gin.Context) {
	// docID 直接来自路由参数，空白清理和业务校验由应用用例统一完成。
	document, err := handler.service.GetDocument(context.Request.Context(), context.Param("docID"))
	if err != nil {
		handler.writeError(context, err)
		return
	}
	// 查询成功后显式转换领域对象，domain 不携带 JSON tag。
	context.JSON(http.StatusOK, Success(documentResponseFromDomain(document)))
}

// listDocuments 读取 current/size 查询参数，并返回兼容分页结构。
func (handler *Handler) listDocuments(context *gin.Context) {
	// current 缺失或非法时沿用兼容默认第一页。
	page := positiveIntQuery(context, "current", 1)
	// size 缺失或非法时沿用每页 20 条。
	size := positiveIntQuery(context, "size", 20)
	// 应用层负责知识库 ID 校验，仓储负责 LIMIT、OFFSET 和总数查询。
	documents, total, err := handler.service.ListDocuments(
		context.Request.Context(),
		context.Param("kbID"),
		page,
		size,
	)
	// 列表或总数查询失败时不返回不一致的分页半结果。
	if err != nil {
		handler.writeError(context, err)
		return
	}

	// 将领域列表逐项转换并连同实际分页参数一起写入兼容结构。
	context.JSON(http.StatusOK, Success(PageResult{
		Records: documentResponsesFromDomain(documents),
		Total:   total,
		Current: page,
		Size:    size,
	}))
}

// deleteDocument 执行删除用例；成功时 data 保持 null，与原接口语义一致。
func (handler *Handler) deleteDocument(context *gin.Context) {
	// 删除用例只接收标准库 Context 和文档 ID，不直接接触 Gin 或数据库。
	if err := handler.service.DeleteDocument(context.Request.Context(), context.Param("docID")); err != nil {
		handler.writeError(context, err)
		return
	}
	// 删除成功时 data 明确为 null，保持旧接口语义。
	context.JSON(http.StatusOK, Success(nil))
}

// writeError 统一输出错误响应，并为服务端故障记录结构化上下文。
func (handler *Handler) writeError(context *gin.Context, err error) {
	// errorResponse 统一决定 HTTP 状态、业务码和客户端可见消息。
	status, result := errorResponse(err)
	// 只有 5xx 才记录完整错误链；已知 4xx 属于客户端可处理问题，避免制造错误日志噪音。
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
	// Abort 阻止后续 Handler 继续执行，并一次性写入 JSON 错误响应。
	context.AbortWithStatusJSON(status, result)
}

// positiveIntQuery 解析正整数查询参数；缺失、格式错误或非正数时使用兼容默认值。
func positiveIntQuery(context *gin.Context, name string, fallback int) int {
	// Query 只读取 URL 查询字符串，不读取表单或 JSON 正文。
	value := context.Query(name)
	// 参数缺失时直接使用调用方提供的兼容默认值。
	if value == "" {
		return fallback
	}
	// Atoi 只接受十进制整数；零、负数和格式错误都回退默认值。
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	// 合法正整数原样返回给应用层。
	return parsed
}

// isNilService 同时识别普通 nil 和接口内部保存的 nil 指针，避免构造成功后第一次调用才 panic。
func isNilService(service KnowledgeService) bool {
	// 普通 nil 接口可以直接判断。
	if service == nil {
		return true
	}
	// ValueOf 取得接口内部的动态值，用于识别有类型的 nil 指针。
	value := reflect.ValueOf(service)
	// 生产实现是指针；只有指针种类才调用 IsNil，避免反射 panic。
	return value.Kind() == reflect.Pointer && value.IsNil()
}
