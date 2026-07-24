package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	httpadapter "github.com/luqingjia/TravelAgent/internal/knowledge/adapter/http"
	"github.com/luqingjia/TravelAgent/internal/knowledge/application"
	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
	"github.com/luqingjia/TravelAgent/internal/platform/config"
	"github.com/luqingjia/TravelAgent/internal/platform/httpserver"
)

// TestNewAppBuildsDependenciesInOrderAndClosesDatabase 验证 app 是唯一组合根，
// 所有具体组件按依赖方向创建，并在 Server 退出后关闭数据库连接池。
func TestNewAppBuildsDependenciesInOrderAndClosesDatabase(t *testing.T) {
	// order 按真实发生顺序保存每个组装步骤，最后用它判断依赖有没有被提前或倒置创建。
	var order []string
	// appendStep 是所有 fake 工厂共用的记录入口，避免每个闭包重复操作切片。
	appendStep := func(step string) { order = append(order, step) }
	// fakeServer 在运行阶段追加记录，用来分清“完成构造”和“真正启动服务”这两个时点。
	fakeServer := &recordingServer{run: func(context.Context) error {
		appendStep("run-server")
		return nil
	}}

	// 每个工厂都返回最小可用对象，同时留下自己的名字，从而完整观察组合根的装配顺序。
	factories := runtimeFactories{
		// 日志器没有上游依赖，所以必须最先创建。
		newLogger: func(config.Log) (*slog.Logger, error) {
			appendStep("logger")
			return slog.New(slog.NewTextHandler(io.Discard, nil)), nil
		},
		// 数据库在日志器之后创建，后续仓储依赖这个连接池。
		openDatabase: func(context.Context, config.Database) (*sqlx.DB, error) {
			appendStep("database")
			return &sqlx.DB{}, nil
		},
		// 关闭函数单独记录，确认它发生在 Server 退出之后。
		closeDatabase: func(*sqlx.DB) error {
			appendStep("close-database")
			return nil
		},
		// 仓储必须拿到已经创建好的数据库对象，但本测试不执行 SQL。
		newRepository: func(*sqlx.DB) (application.DocumentRepository, error) {
			appendStep("repository")
			return noopRepository{}, nil
		},
		// 存储和 Embedding 都是应用服务的外部端口实现，彼此之间没有依赖。
		newStorage: func(context.Context, config.Storage) (application.ObjectStorage, error) {
			appendStep("storage")
			return noopStorage{}, nil
		},
		newEmbedder: func(config.Embedding) (application.Embedder, error) {
			appendStep("embedder")
			return noopEmbedder{}, nil
		},
		// 应用服务只能在仓储、存储和 Embedding 都准备好之后创建。
		newService: func(application.Dependencies) (httpadapter.KnowledgeService, error) {
			appendStep("service")
			return noopKnowledgeService{}, nil
		},
		// Handler、中间件、路由和 Server 按 HTTP 调用链从内向外完成装配。
		newHandler: func(service httpadapter.KnowledgeService, logger *slog.Logger) (*httpadapter.Handler, error) {
			appendStep("handler")
			return httpadapter.NewHandler(service, logger)
		},
		newMiddleware: func(logger *slog.Logger) (httpserver.Middleware, error) {
			appendStep("middleware")
			return httpserver.NewMiddleware(logger)
		},
		newRouter: func(*httpadapter.Handler, httpserver.Middleware) http.Handler {
			appendStep("router")
			return http.NotFoundHandler()
		},
		newServer: func(config.HTTP, http.Handler, *slog.Logger) (serverRunner, error) {
			appendStep("server")
			return fakeServer, nil
		},
	}

	// 先执行组装；如果任意工厂顺序不对或返回错误，测试在这里直接失败。
	runtime, err := newAppWithFactories(t.Context(), validAppConfig(), factories)
	if err != nil {
		t.Fatalf("newAppWithFactories() error = %v", err)
	}
	// 再进入运行阶段，Server 返回后 App 应执行数据库收尾。
	if err := runtime.Run(t.Context()); err != nil {
		t.Fatalf("App.Run() error = %v", err)
	}

	// want 描述项目约定的完整生命周期；数据库关闭必须是最后一步。
	want := []string{
		"logger", "database", "repository", "storage", "embedder",
		"service", "handler", "middleware", "router", "server",
		"run-server", "close-database",
	}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("assembly order = %v, want %v", order, want)
	}
}

// TestNewAppClosesDatabaseWhenLaterConstructionFails 验证数据库建立后若存储构造失败，
// 已创建资源会立即清理，并且返回最初的存储错误。
func TestNewAppClosesDatabaseWhenLaterConstructionFails(t *testing.T) {
	// storageError 代表数据库已经连接成功后，下一项长期依赖创建失败。
	storageError := errors.New("storage unavailable")
	// closeCalls 用于证明清理动作只执行一次，没有遗漏也没有重复关闭。
	closeCalls := 0
	// 从一套能成功组装的工厂出发，只替换本场景需要的失败点和清理记录器。
	factories := minimalFactories()
	factories.newStorage = func(context.Context, config.Storage) (application.ObjectStorage, error) {
		return nil, storageError
	}
	factories.closeDatabase = func(*sqlx.DB) error {
		closeCalls++
		return nil
	}

	// 执行组装后应直接得到存储错误，Server 不会被创建和运行。
	_, err := newAppWithFactories(t.Context(), validAppConfig(), factories)
	// 返回值必须保留原始错误链，方便入口层判断和日志定位。
	if !errors.Is(err, storageError) {
		t.Fatalf("newAppWithFactories() error = %v, want storage error", err)
	}
	// 数据库是失败前唯一已经拿到的外部资源，所以必须立即关闭一次。
	if closeCalls != 1 {
		t.Fatalf("database close calls = %d, want 1", closeCalls)
	}
}

// minimalFactories 返回能完成组装的轻量 fake 工厂，单个测试只覆盖自己替换的失败点。
func minimalFactories() runtimeFactories {
	// 日志输出丢弃，避免 fake 组装过程污染测试终端。
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// 所有工厂都返回最小实现，让调用方只替换自己关心的一个环节。
	return runtimeFactories{
		newLogger:     func(config.Log) (*slog.Logger, error) { return logger, nil },
		openDatabase:  func(context.Context, config.Database) (*sqlx.DB, error) { return &sqlx.DB{}, nil },
		closeDatabase: func(*sqlx.DB) error { return nil },
		newRepository: func(*sqlx.DB) (application.DocumentRepository, error) { return noopRepository{}, nil },
		newStorage:    func(context.Context, config.Storage) (application.ObjectStorage, error) { return noopStorage{}, nil },
		newEmbedder:   func(config.Embedding) (application.Embedder, error) { return noopEmbedder{}, nil },
		newService: func(application.Dependencies) (httpadapter.KnowledgeService, error) {
			return noopKnowledgeService{}, nil
		},
		newHandler:    httpadapter.NewHandler,
		newMiddleware: httpserver.NewMiddleware,
		newRouter:     func(*httpadapter.Handler, httpserver.Middleware) http.Handler { return http.NotFoundHandler() },
		newServer:     func(config.HTTP, http.Handler, *slog.Logger) (serverRunner, error) { return &recordingServer{}, nil },
	}
}

// validAppConfig 构造一份不连接外部服务但能通过集中校验的配置。
func validAppConfig() config.Config {
	// 这些值只用于通过构造校验，不会真的连接示例地址或监听固定端口。
	return config.Config{
		HTTP: config.HTTP{
			Port:              "8081",
			ReadHeaderTimeout: time.Second,
			ReadTimeout:       time.Second,
			WriteTimeout:      time.Minute,
			IdleTimeout:       time.Second,
			ShutdownTimeout:   time.Second,
		},
		Database: config.Database{
			DSN:                "postgres://example",
			MaxOpenConnections: 10,
			MaxIdleConnections: 5,
			ConnMaxLifetime:    time.Minute,
			ConnMaxIdleTime:    time.Minute,
		},
		Storage: config.Storage{S3Enabled: false, LocalDir: ".data/test"},
		Embedding: config.Embedding{
			APIKey:     "test-key",
			BaseURL:    "https://example.com",
			Model:      "model",
			Dimensions: application.EmbeddingDimensions,
			Timeout:    time.Second,
		},
		Document: config.Document{
			AllowedExtensions: map[string]struct{}{"txt": {}},
			MaxUploadBytes:    1024,
		},
		Log: config.Log{Level: "info", Format: "json"},
	}
}

// recordingServer 记录组合根是否真正调用了运行阶段。
type recordingServer struct {
	// run 保存测试希望在 Server 运行时触发的行为；为空表示直接成功退出。
	run func(context.Context) error
}

// Run 模拟真实 Server 的生命周期入口，并把行为交给测试注入的闭包。
func (server *recordingServer) Run(ctx context.Context) error {
	// 没有注入行为时按成功处理，方便不关心运行阶段的组装测试复用。
	if server.run == nil {
		return nil
	}
	return server.run(ctx)
}

// noopRepository 实现应用层仓储端口；组合根测试不执行任何业务 SQL。
type noopRepository struct{}

// KnowledgeBaseExists 返回存在，避免组合根测试被无关的知识库校验阻断。
func (noopRepository) KnowledgeBaseExists(context.Context, string) (bool, error) { return true, nil }

// ActiveDocumentHashExists 返回不重复；本测试只要求接口完整，不验证去重逻辑。
func (noopRepository) ActiveDocumentHashExists(context.Context, string, string) (bool, error) {
	return false, nil
}

// CreateDocument 模拟文档写入成功，不接触真实数据库。
func (noopRepository) CreateDocument(context.Context, domain.Document) error { return nil }

// GetDocument 返回空结果，满足查询端口的函数签名。
func (noopRepository) GetDocument(context.Context, string) (domain.Document, error) {
	return domain.Document{}, nil
}

// ListDocuments 返回空页，组合根测试不关心分页内容。
func (noopRepository) ListDocuments(context.Context, string, int, int) ([]domain.Document, int64, error) {
	return nil, 0, nil
}

// DeleteDocument 模拟删除成功。
func (noopRepository) DeleteDocument(context.Context, string) error { return nil }

// TryMarkProcessing 返回未抢占，避免构造测试意外进入处理流程。
func (noopRepository) TryMarkProcessing(context.Context, string) (domain.Document, bool, error) {
	return domain.Document{}, false, nil
}

// ReplaceDocumentChunks 模拟原子替换成功。
func (noopRepository) ReplaceDocumentChunks(context.Context, domain.Document, []domain.Chunk, [][]float32) error {
	return nil
}

// MarkFailed 模拟失败状态回写成功。
func (noopRepository) MarkFailed(context.Context, domain.Document) error { return nil }

// noopStorage 和 noopEmbedder 只用于证明接口依赖能被显式传入。
type noopStorage struct{}

// Put 模拟对象写入成功并返回空的稳定对象。
func (noopStorage) Put(context.Context, application.StoredObjectInput) (application.StoredObject, error) {
	return application.StoredObject{}, nil
}

// Get 模拟读取到空内容。
func (noopStorage) Get(context.Context, string) ([]byte, error) { return nil, nil }

// Delete 模拟对象删除成功。
func (noopStorage) Delete(context.Context, string) error { return nil }

// noopEmbedder 是不访问外部模型的 Embedding 端口实现。
type noopEmbedder struct{}

// EmbedTexts 返回空向量批次，组合根测试只关心依赖能否注入。
func (noopEmbedder) EmbedTexts(context.Context, []string) ([][]float32, error) { return nil, nil }

// noopKnowledgeService 实现 HTTP 用例接口，组合根测试不关心具体业务返回值。
type noopKnowledgeService struct{}

// UploadDocument 模拟上传用例成功。
func (noopKnowledgeService) UploadDocument(context.Context, application.UploadInput) (domain.Document, error) {
	return domain.Document{}, nil
}

// ProcessDocument 模拟分块处理用例成功。
func (noopKnowledgeService) ProcessDocument(context.Context, string, domain.ChunkOptions) (domain.Document, error) {
	return domain.Document{}, nil
}

// GetDocument 模拟单文档查询成功。
func (noopKnowledgeService) GetDocument(context.Context, string) (domain.Document, error) {
	return domain.Document{}, nil
}

// ListDocuments 模拟返回空分页结果。
func (noopKnowledgeService) ListDocuments(context.Context, string, int, int) ([]domain.Document, int64, error) {
	return nil, 0, nil
}

// DeleteDocument 模拟删除用例成功。
func (noopKnowledgeService) DeleteDocument(context.Context, string) error { return nil }
