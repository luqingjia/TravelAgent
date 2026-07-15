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
	var order []string
	appendStep := func(step string) { order = append(order, step) }
	fakeServer := &recordingServer{run: func(context.Context) error {
		appendStep("run-server")
		return nil
	}}

	factories := runtimeFactories{
		newLogger: func(config.Log) (*slog.Logger, error) {
			appendStep("logger")
			return slog.New(slog.NewTextHandler(io.Discard, nil)), nil
		},
		openDatabase: func(context.Context, config.Database) (*sqlx.DB, error) {
			appendStep("database")
			return &sqlx.DB{}, nil
		},
		closeDatabase: func(*sqlx.DB) error {
			appendStep("close-database")
			return nil
		},
		newRepository: func(*sqlx.DB) (application.DocumentRepository, error) {
			appendStep("repository")
			return noopRepository{}, nil
		},
		newStorage: func(context.Context, config.Storage) (application.ObjectStorage, error) {
			appendStep("storage")
			return noopStorage{}, nil
		},
		newEmbedder: func(config.Embedding) (application.Embedder, error) {
			appendStep("embedder")
			return noopEmbedder{}, nil
		},
		newService: func(application.Dependencies) (httpadapter.KnowledgeService, error) {
			appendStep("service")
			return noopKnowledgeService{}, nil
		},
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

	runtime, err := newAppWithFactories(t.Context(), validAppConfig(), factories)
	if err != nil {
		t.Fatalf("newAppWithFactories() error = %v", err)
	}
	if err := runtime.Run(t.Context()); err != nil {
		t.Fatalf("App.Run() error = %v", err)
	}

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
	storageError := errors.New("storage unavailable")
	closeCalls := 0
	factories := minimalFactories()
	factories.newStorage = func(context.Context, config.Storage) (application.ObjectStorage, error) {
		return nil, storageError
	}
	factories.closeDatabase = func(*sqlx.DB) error {
		closeCalls++
		return nil
	}

	_, err := newAppWithFactories(t.Context(), validAppConfig(), factories)
	if !errors.Is(err, storageError) {
		t.Fatalf("newAppWithFactories() error = %v, want storage error", err)
	}
	if closeCalls != 1 {
		t.Fatalf("database close calls = %d, want 1", closeCalls)
	}
}

// minimalFactories 返回能完成组装的轻量 fake 工厂，单个测试只覆盖自己替换的失败点。
func minimalFactories() runtimeFactories {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
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
	run func(context.Context) error
}

func (server *recordingServer) Run(ctx context.Context) error {
	if server.run == nil {
		return nil
	}
	return server.run(ctx)
}

// noopRepository 实现应用层仓储端口；组合根测试不执行任何业务 SQL。
type noopRepository struct{}

func (noopRepository) KnowledgeBaseExists(context.Context, string) (bool, error) { return true, nil }
func (noopRepository) ActiveDocumentHashExists(context.Context, string, string) (bool, error) {
	return false, nil
}
func (noopRepository) CreateDocument(context.Context, domain.Document) error { return nil }
func (noopRepository) GetDocument(context.Context, string) (domain.Document, error) {
	return domain.Document{}, nil
}
func (noopRepository) ListDocuments(context.Context, string, int, int) ([]domain.Document, int64, error) {
	return nil, 0, nil
}
func (noopRepository) DeleteDocument(context.Context, string) error { return nil }
func (noopRepository) TryMarkProcessing(context.Context, string) (domain.Document, bool, error) {
	return domain.Document{}, false, nil
}
func (noopRepository) ReplaceDocumentChunks(context.Context, domain.Document, []domain.Chunk, [][]float32) error {
	return nil
}
func (noopRepository) MarkFailed(context.Context, domain.Document) error { return nil }

// noopStorage 和 noopEmbedder 只用于证明接口依赖能被显式传入。
type noopStorage struct{}

func (noopStorage) Put(context.Context, application.StoredObjectInput) (application.StoredObject, error) {
	return application.StoredObject{}, nil
}
func (noopStorage) Get(context.Context, string) ([]byte, error) { return nil, nil }
func (noopStorage) Delete(context.Context, string) error        { return nil }

type noopEmbedder struct{}

func (noopEmbedder) EmbedTexts(context.Context, []string) ([][]float32, error) { return nil, nil }

// noopKnowledgeService 实现 HTTP 用例接口，组合根测试不关心具体业务返回值。
type noopKnowledgeService struct{}

func (noopKnowledgeService) UploadDocument(context.Context, application.UploadInput) (domain.Document, error) {
	return domain.Document{}, nil
}
func (noopKnowledgeService) ProcessDocument(context.Context, string, domain.ChunkOptions) (domain.Document, error) {
	return domain.Document{}, nil
}
func (noopKnowledgeService) GetDocument(context.Context, string) (domain.Document, error) {
	return domain.Document{}, nil
}
func (noopKnowledgeService) ListDocuments(context.Context, string, int, int) ([]domain.Document, int64, error) {
	return nil, 0, nil
}
func (noopKnowledgeService) DeleteDocument(context.Context, string) error { return nil }
