package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"

	agenthttp "github.com/luqingjia/TravelAgent/internal/agent/adapter/http"
	agentapp "github.com/luqingjia/TravelAgent/internal/agent/application"
	"github.com/luqingjia/TravelAgent/internal/agent/domain"
	httpadapter "github.com/luqingjia/TravelAgent/internal/knowledge/adapter/http"
	"github.com/luqingjia/TravelAgent/internal/knowledge/application"
	knowledgedomain "github.com/luqingjia/TravelAgent/internal/knowledge/domain"
	"github.com/luqingjia/TravelAgent/internal/platform/config"
	"github.com/luqingjia/TravelAgent/internal/platform/httpserver"
)

// TestNewAppWithFactoriesClosesDatabaseWhenAgentRuntimeFails 验证数据库打开后 Agent 运行时构造失败会关闭连接池。
func TestNewAppWithFactoriesClosesDatabaseWhenAgentRuntimeFails(t *testing.T) {
	order := &callOrder{}
	closed := false
	factories := baseTestFactories(order)
	factories.openDatabase = func(context.Context, config.Database) (*sqlx.DB, error) {
		order.add("openDatabase")
		// sqlx.DB 零值足够作为“已打开资源”的句柄；测试只关心关闭回调被调用。
		return &sqlx.DB{}, nil
	}
	factories.closeDatabase = func(*sqlx.DB) error {
		order.add("closeDatabase")
		closed = true
		return nil
	}
	factories.newAgentRuntime = func(context.Context, config.Agent) (agentapp.AgentRuntime, agentapp.ModelCatalog, error) {
		order.add("newAgentRuntime")
		return nil, nil, errors.New("agent runtime boom")
	}

	_, err := newAppWithFactories(context.Background(), validTestConfig(), factories)
	if err == nil || !strings.Contains(err.Error(), "create agent runtime") {
		t.Fatalf("error = %v, want agent runtime failure", err)
	}
	if !closed {
		t.Fatal("database was not closed after agent runtime failure")
	}
	if !order.hasSequence("openDatabase", "newRepository", "newStorage", "newEmbedder", "newService", "newHandler", "newAgentRuntime", "closeDatabase") {
		// Agent 在 knowledge handler 之后；失败时必须关闭已打开数据库。
		t.Fatalf("call order = %v", order.steps)
	}
}

// TestNewAppWithFactoriesBuildsAgentAfterKnowledge 验证成功路径中 Agent 构造顺序位于知识上下文之后、Router 之前。
func TestNewAppWithFactoriesBuildsAgentAfterKnowledge(t *testing.T) {
	order := &callOrder{}
	factories := baseTestFactories(order)
	factories.openDatabase = func(context.Context, config.Database) (*sqlx.DB, error) {
		order.add("openDatabase")
		return &sqlx.DB{}, nil
	}
	factories.closeDatabase = func(*sqlx.DB) error {
		order.add("closeDatabase")
		return nil
	}

	runtime, err := newAppWithFactories(context.Background(), validTestConfig(), factories)
	if err != nil {
		t.Fatalf("newAppWithFactories() error = %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	if !order.hasSequence(
		"openDatabase",
		"newRepository",
		"newStorage",
		"newEmbedder",
		"newService",
		"newHandler",
		"newAgentRuntime",
		"newAgentService",
		"newAgentHandler",
		"newMiddleware",
		"newRouter",
		"newServer",
	) {
		t.Fatalf("call order = %v", order.steps)
	}
}

type callOrder struct {
	mu    sync.Mutex
	steps []string
}

func (order *callOrder) add(step string) {
	order.mu.Lock()
	defer order.mu.Unlock()
	order.steps = append(order.steps, step)
}

func (order *callOrder) hasPrefix(want ...string) bool {
	order.mu.Lock()
	defer order.mu.Unlock()
	if len(order.steps) < len(want) {
		return false
	}
	for i := range want {
		if order.steps[i] != want[i] {
			return false
		}
	}
	return true
}

func (order *callOrder) hasSequence(want ...string) bool {
	order.mu.Lock()
	defer order.mu.Unlock()
	index := 0
	for _, step := range order.steps {
		if step == want[index] {
			index++
			if index == len(want) {
				return true
			}
		}
	}
	return false
}

func validTestConfig() config.Config {
	return config.Config{
		HTTP: config.HTTP{
			Port:              "8081",
			ReadHeaderTimeout: 1,
			ReadTimeout:       1,
			WriteTimeout:      1,
			IdleTimeout:       1,
			ShutdownTimeout:   1,
		},
		Database: config.Database{
			DSN:                "postgres://travelagent:travelagent@localhost:5432/kenagent?sslmode=disable",
			MaxOpenConnections: 1,
			MaxIdleConnections: 1,
			ConnMaxLifetime:    1,
			ConnMaxIdleTime:    1,
		},
		Storage: config.Storage{
			S3Enabled: false,
			LocalDir:  ".data/storage-test",
		},
		Embedding: config.Embedding{
			APIKey:     "replace-me",
			BaseURL:    "https://example.com",
			Model:      "text-embedding-v3",
			Dimensions: 1536,
			Timeout:    1,
		},
		Document: config.Document{
			MaxUploadBytes:    1024,
			AllowedExtensions: map[string]struct{}{".txt": {}},
		},
		Agent: config.Agent{
			DefaultModelID:     "qwen-plus",
			MaxHistoryMessages: 20,
			MaxMessageChars:    4000,
			MaxTotalChars:      16000,
			MaxIterations:      8,
			Models: []config.AgentModel{{
				ID: "qwen-plus", DisplayName: "Qwen Plus", Provider: "dashscope",
				BaseURL: "https://example.com/v1", Model: "qwen-plus", APIKey: "replace-me",
				Timeout: 1, Enabled: true,
			}},
		},
		Log: config.Log{Level: "info", Format: "text"},
	}
}

func baseTestFactories(order *callOrder) runtimeFactories {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return runtimeFactories{
		newLogger: func(config.Log) (*slog.Logger, error) {
			order.add("newLogger")
			return logger, nil
		},
		openDatabase: func(context.Context, config.Database) (*sqlx.DB, error) {
			order.add("openDatabase")
			return &sqlx.DB{}, nil
		},
		closeDatabase: func(*sqlx.DB) error {
			order.add("closeDatabase")
			return nil
		},
		newRepository: func(*sqlx.DB) (application.DocumentRepository, error) {
			order.add("newRepository")
			return &fakeDocumentRepository{}, nil
		},
		newStorage: func(context.Context, config.Storage) (application.ObjectStorage, error) {
			order.add("newStorage")
			return &fakeObjectStorage{}, nil
		},
		newEmbedder: func(config.Embedding) (application.Embedder, error) {
			order.add("newEmbedder")
			return &fakeEmbedder{}, nil
		},
		newService: func(application.Dependencies) (httpadapter.KnowledgeService, error) {
			order.add("newService")
			return &fakeKnowledgeService{}, nil
		},
		newHandler: func(httpadapter.KnowledgeService, *slog.Logger) (*httpadapter.Handler, error) {
			order.add("newHandler")
			return httpadapter.NewHandler(&fakeKnowledgeService{}, logger)
		},
		newAgentRuntime: func(context.Context, config.Agent) (agentapp.AgentRuntime, agentapp.ModelCatalog, error) {
			order.add("newAgentRuntime")
			catalog := &staticCatalog{view: domain.CatalogView{DefaultModelID: "qwen-plus"}}
			return &fakeAgentRuntime{}, catalog, nil
		},
		newAgentService: func(agentapp.Dependencies) (agenthttp.AgentService, error) {
			order.add("newAgentService")
			return &fakeAgentService{}, nil
		},
		newAgentHandler: func(agenthttp.AgentService, *slog.Logger) (*agenthttp.Handler, error) {
			order.add("newAgentHandler")
			return agenthttp.NewHandler(&fakeAgentService{}, logger)
		},
		newMiddleware: func(*slog.Logger) (httpserver.Middleware, error) {
			order.add("newMiddleware")
			return httpserver.NewMiddleware(logger)
		},
		newRouter: func(*httpadapter.Handler, *agenthttp.Handler, httpserver.Middleware) http.Handler {
			order.add("newRouter")
			return http.NotFoundHandler()
		},
		newServer: func(config.HTTP, http.Handler, *slog.Logger) (serverRunner, error) {
			order.add("newServer")
			return &fakeServer{}, nil
		},
	}
}

type staticCatalog struct {
	view domain.CatalogView
}

func (catalog *staticCatalog) List() domain.CatalogView { return catalog.view }

type fakeAgentRuntime struct{}

func (runtime *fakeAgentRuntime) Chat(context.Context, string, []domain.Message) (domain.AssistantMessage, error) {
	return domain.AssistantMessage{}, nil
}
func (runtime *fakeAgentRuntime) Stream(context.Context, string, []domain.Message) (<-chan agentapp.StreamEvent, error) {
	out := make(chan agentapp.StreamEvent)
	close(out)
	return out, nil
}

type fakeAgentService struct{}

func (service *fakeAgentService) ListModels() domain.CatalogView { return domain.CatalogView{} }
func (service *fakeAgentService) Chat(context.Context, string, []domain.Message) (domain.AssistantMessage, error) {
	return domain.AssistantMessage{}, nil
}
func (service *fakeAgentService) Stream(context.Context, string, []domain.Message) (<-chan agentapp.StreamEvent, error) {
	out := make(chan agentapp.StreamEvent)
	close(out)
	return out, nil
}

type fakeKnowledgeService struct{}

func (service *fakeKnowledgeService) UploadDocument(context.Context, application.UploadInput) (knowledgedomain.Document, error) {
	return knowledgedomain.Document{}, nil
}
func (service *fakeKnowledgeService) ProcessDocument(context.Context, string, knowledgedomain.ChunkOptions) (knowledgedomain.Document, error) {
	return knowledgedomain.Document{}, nil
}
func (service *fakeKnowledgeService) GetDocument(context.Context, string) (knowledgedomain.Document, error) {
	return knowledgedomain.Document{}, nil
}
func (service *fakeKnowledgeService) ListDocuments(context.Context, string, int, int) ([]knowledgedomain.Document, int64, error) {
	return nil, 0, nil
}
func (service *fakeKnowledgeService) DeleteDocument(context.Context, string) error { return nil }

type fakeDocumentRepository struct{}

func (repository *fakeDocumentRepository) KnowledgeBaseExists(context.Context, string) (bool, error) {
	return true, nil
}
func (repository *fakeDocumentRepository) ActiveDocumentHashExists(context.Context, string, string) (bool, error) {
	return false, nil
}
func (repository *fakeDocumentRepository) CreateDocument(context.Context, knowledgedomain.Document) error {
	return nil
}
func (repository *fakeDocumentRepository) GetDocument(context.Context, string) (knowledgedomain.Document, error) {
	return knowledgedomain.Document{}, nil
}
func (repository *fakeDocumentRepository) ListDocuments(context.Context, string, int, int) ([]knowledgedomain.Document, int64, error) {
	return nil, 0, nil
}
func (repository *fakeDocumentRepository) DeleteDocument(context.Context, string) error { return nil }
func (repository *fakeDocumentRepository) TryMarkProcessing(context.Context, string) (knowledgedomain.Document, bool, error) {
	return knowledgedomain.Document{}, false, nil
}
func (repository *fakeDocumentRepository) ReplaceDocumentChunks(context.Context, knowledgedomain.Document, []knowledgedomain.Chunk, [][]float32) error {
	return nil
}
func (repository *fakeDocumentRepository) MarkFailed(context.Context, knowledgedomain.Document) error {
	return nil
}

type fakeObjectStorage struct{}

func (storage *fakeObjectStorage) Put(context.Context, application.StoredObjectInput) (application.StoredObject, error) {
	return application.StoredObject{}, nil
}
func (storage *fakeObjectStorage) Get(context.Context, string) ([]byte, error) {
	return []byte(""), nil
}
func (storage *fakeObjectStorage) Delete(context.Context, string) error { return nil }

type fakeEmbedder struct{}

func (embedder *fakeEmbedder) EmbedTexts(context.Context, []string) ([][]float32, error) {
	return nil, nil
}

type fakeServer struct{}

func (server *fakeServer) Run(context.Context) error { return nil }
