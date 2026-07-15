// Package app 是 TravelAgent 的唯一组合根，负责创建具体依赖并管理进程级资源生命周期。
//
// 这里可以同时看见数据库、对象存储、Embedding、应用服务和 HTTP 适配器，因为“把所有零件
// 接起来”正是组合根的职责。其他业务包只依赖自己需要的小接口，不能反过来导入 app，也不能
// 自己读取环境变量或偷偷创建全局单例。
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/jmoiron/sqlx"

	httpadapter "github.com/luqingjia/TravelAgent/internal/knowledge/adapter/http"
	postgresadapter "github.com/luqingjia/TravelAgent/internal/knowledge/adapter/postgres"
	"github.com/luqingjia/TravelAgent/internal/knowledge/application"
	"github.com/luqingjia/TravelAgent/internal/platform/config"
	"github.com/luqingjia/TravelAgent/internal/platform/database"
	"github.com/luqingjia/TravelAgent/internal/platform/embedding"
	"github.com/luqingjia/TravelAgent/internal/platform/httpserver"
	platformstorage "github.com/luqingjia/TravelAgent/internal/platform/storage"
)

// serverRunner 是组合根对 HTTP 生命周期的最小要求。
//
// 生产环境传入 *Server；测试传入不会监听真实端口的 fake。接口放在使用方 app 包里，说明 app
// 只关心“这个对象能一直运行到 context 被取消”，并不需要知道 http.Server 的内部细节。
type serverRunner interface {
	Run(context.Context) error
}

// runtimeFactories 集中保存每个具体依赖的构造函数。
//
// 它不是运行时 DI 容器：生产仍按 newAppWithFactories 中写死的顺序手工组装。这个内部结构只让
// 单元测试能够替换会连接 PostgreSQL、S3 或模型平台的构造步骤，从而验证创建顺序和失败清理。
type runtimeFactories struct {
	newLogger     func(config.Log) (*slog.Logger, error)
	openDatabase  func(context.Context, config.Database) (*sqlx.DB, error)
	closeDatabase func(*sqlx.DB) error
	newRepository func(*sqlx.DB) (application.DocumentRepository, error)
	newStorage    func(context.Context, config.Storage) (application.ObjectStorage, error)
	newEmbedder   func(config.Embedding) (application.Embedder, error)
	newService    func(application.Dependencies) (httpadapter.KnowledgeService, error)
	newHandler    func(httpadapter.KnowledgeService, *slog.Logger) (*httpadapter.Handler, error)
	newMiddleware func(*slog.Logger) (httpserver.Middleware, error)
	newRouter     func(*httpadapter.Handler, httpserver.Middleware) http.Handler
	newServer     func(config.HTTP, http.Handler, *slog.Logger) (serverRunner, error)
}

// App 保存服务运行期间真正需要长期持有的进程级资源。
//
// Handler、Repository 等对象已经通过引用链被 Server 持有，不需要在这里重复保存。数据库连接池
// 必须显式保留，因为它需要在 HTTP 服务退出后关闭。
type App struct {
	server        serverRunner
	database      *sqlx.DB
	closeDatabase func(*sqlx.DB) error
	logger        *slog.Logger

	// Close 可能被正常退出路径和错误兜底路径同时调用。sync.Once 保证连接池最多关闭一次。
	closeOnce sync.Once
	closeErr  error
}

// Run 是 cmd/travel-agent 调用的进程级入口。
//
// 固定顺序是：读取环境变量、集中校验并组装全部依赖、设置默认结构化日志器，最后进入 HTTP
// 生命周期。任何一步失败都会把带上下文的错误交给 main 决定退出码。
func Run(ctx context.Context) error {
	configuration, err := config.Load(os.Getenv)
	if err != nil {
		return fmt.Errorf("load application configuration: %w", err)
	}

	runtime, err := New(ctx, configuration)
	if err != nil {
		return err
	}

	// 只有完整组装成功后才替换默认日志器，避免半初始化状态影响同进程中的其他代码或测试。
	slog.SetDefault(runtime.logger)
	return runtime.Run(ctx)
}

// New 使用生产构造函数建立一套完整 App，但暂不开始监听端口。
//
// 把“创建”和“运行”分开后，调用方可以先拿到完整对象；如果后续增加启动探针，也不会被迫把
// 所有逻辑塞进 main。当前配置会在任何外部连接建立前完成校验。
func New(ctx context.Context, configuration config.Config) (*App, error) {
	return newAppWithFactories(ctx, configuration, defaultRuntimeFactories())
}

// Run 启动 HTTP 服务，并确保服务无论正常返回还是报错都关闭数据库连接池。
func (app *App) Run(ctx context.Context) error {
	if app == nil || app.server == nil {
		return fmt.Errorf("application server is required")
	}

	runErr := app.server.Run(ctx)
	closeErr := app.Close()

	// errors.Join 会保留两个错误链：既不吞掉真正的运行错误，也不会隐藏连接池关闭失败。
	// errors.Is/errors.As 仍能沿链识别原始错误，调用方不需要依赖错误字符串。
	switch {
	case runErr != nil && closeErr != nil:
		return errors.Join(
			fmt.Errorf("run HTTP server: %w", runErr),
			fmt.Errorf("close application resources: %w", closeErr),
		)
	case runErr != nil:
		return fmt.Errorf("run HTTP server: %w", runErr)
	case closeErr != nil:
		return fmt.Errorf("close application resources: %w", closeErr)
	default:
		return nil
	}
}

// Close 释放 App 自己拥有的数据库连接池，并且可以安全地重复调用。
func (app *App) Close() error {
	if app == nil {
		return nil
	}

	app.closeOnce.Do(func() {
		if app.database == nil || app.closeDatabase == nil {
			return
		}
		app.closeErr = app.closeDatabase(app.database)
	})
	return app.closeErr
}

// newAppWithFactories 按依赖方向手工创建整张运行对象图。
//
// 顺序不能随意调换：Repository 需要数据库，Service 需要 Repository/Storage/Embedder，Handler
// 需要 Service，Router 和 Server 又依赖前面的 HTTP 对象。数据库一旦建立，后面任何步骤失败都
// 必须立刻关闭它，避免启动重试时泄漏连接池。
func newAppWithFactories(
	ctx context.Context,
	configuration config.Config,
	factories runtimeFactories,
) (*App, error) {
	// 集中校验发生在第一个外部连接之前，因此缺少密钥或超时写错时不会留下半启动资源。
	if err := configuration.Validate(); err != nil {
		return nil, fmt.Errorf("validate application configuration: %w", err)
	}

	logger, err := factories.newLogger(configuration.Log)
	if err != nil {
		return nil, fmt.Errorf("create application logger: %w", err)
	}
	if logger == nil {
		return nil, fmt.Errorf("create application logger: constructor returned nil")
	}

	databaseConnection, err := factories.openDatabase(ctx, configuration.Database)
	if err != nil {
		return nil, fmt.Errorf("open application database: %w", err)
	}
	if databaseConnection == nil {
		return nil, fmt.Errorf("open application database: constructor returned nil")
	}

	// cleanupConstructionError 是数据库建立后的统一失败出口。清理失败会作为第二条错误链加入，
	// 但最先发生的构造错误仍用 %w 保留，errors.Is 仍然能准确识别它。
	cleanupConstructionError := func(step string, constructionErr error) error {
		primaryErr := fmt.Errorf("%s: %w", step, constructionErr)
		if factories.closeDatabase == nil {
			return errors.Join(primaryErr, fmt.Errorf("close database after construction failure: close function is nil"))
		}
		if closeErr := factories.closeDatabase(databaseConnection); closeErr != nil {
			return errors.Join(
				primaryErr,
				fmt.Errorf("close database after construction failure: %w", closeErr),
			)
		}
		return primaryErr
	}

	repository, err := factories.newRepository(databaseConnection)
	if err != nil {
		return nil, cleanupConstructionError("create document repository", err)
	}

	objectStorage, err := factories.newStorage(ctx, configuration.Storage)
	if err != nil {
		return nil, cleanupConstructionError("create object storage", err)
	}

	embedder, err := factories.newEmbedder(configuration.Embedding)
	if err != nil {
		return nil, cleanupConstructionError("create embedding client", err)
	}

	service, err := factories.newService(application.Dependencies{
		Repository: repository,
		Storage:    objectStorage,
		Embedder:   embedder,
		Policy: application.UploadPolicy{
			MaxUploadBytes:    configuration.Document.MaxUploadBytes,
			AllowedExtensions: configuration.Document.AllowedExtensions,
		},
		Logger: logger,
	})
	if err != nil {
		return nil, cleanupConstructionError("create knowledge application service", err)
	}

	handler, err := factories.newHandler(service, logger)
	if err != nil {
		return nil, cleanupConstructionError("create knowledge HTTP handler", err)
	}

	middleware, err := factories.newMiddleware(logger)
	if err != nil {
		return nil, cleanupConstructionError("create HTTP middleware", err)
	}

	// Router 本身没有需要关闭的资源，它只是把已经构造好的 Handler 和中间件注册到路由树。
	router := factories.newRouter(handler, middleware)
	server, err := factories.newServer(configuration.HTTP, router, logger)
	if err != nil {
		return nil, cleanupConstructionError("create HTTP server", err)
	}

	return &App{
		server:        server,
		database:      databaseConnection,
		closeDatabase: factories.closeDatabase,
		logger:        logger,
	}, nil
}

// defaultRuntimeFactories 把每个生产构造函数适配成组合根需要的统一签名。
func defaultRuntimeFactories() runtimeFactories {
	return runtimeFactories{
		newLogger:     newLogger,
		openDatabase:  database.Open,
		closeDatabase: func(database *sqlx.DB) error { return database.Close() },
		newRepository: func(database *sqlx.DB) (application.DocumentRepository, error) {
			return postgresadapter.NewRepository(database)
		},
		newStorage: func(ctx context.Context, configuration config.Storage) (application.ObjectStorage, error) {
			if configuration.S3Enabled {
				return platformstorage.NewS3Storage(ctx, configuration)
			}
			return platformstorage.NewLocalStorage(configuration.LocalDir)
		},
		newEmbedder: func(configuration config.Embedding) (application.Embedder, error) {
			return embedding.NewClient(configuration)
		},
		newService: func(dependencies application.Dependencies) (httpadapter.KnowledgeService, error) {
			return application.NewService(dependencies)
		},
		newHandler:    httpadapter.NewHandler,
		newMiddleware: httpserver.NewMiddleware,
		newRouter: func(handler *httpadapter.Handler, middleware httpserver.Middleware) http.Handler {
			return httpadapter.NewRouter(handler, middleware)
		},
		newServer: func(configuration config.HTTP, handler http.Handler, logger *slog.Logger) (serverRunner, error) {
			return NewServer(configuration, handler, logger)
		},
	}
}

// newLogger 根据配置创建标准库 slog 日志器。
// JSON 适合生产日志采集，text 便于本地阅读；两种格式共享同一个等级过滤规则。
func newLogger(configuration config.Log) (*slog.Logger, error) {
	var level slog.Level
	switch configuration.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		return nil, fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, error")
	}

	options := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	switch configuration.Format {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, options)
	case "text":
		handler = slog.NewTextHandler(os.Stdout, options)
	default:
		return nil, fmt.Errorf("LOG_FORMAT must be json or text")
	}
	return slog.New(handler), nil
}
