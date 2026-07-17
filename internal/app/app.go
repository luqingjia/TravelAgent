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
	// Run 一直运行到监听失败、请求服务失败或调用方 Context 被取消。
	Run(context.Context) error
}

// runtimeFactories 集中保存每个具体依赖的构造函数。
//
// 它不是运行时 DI 容器：生产仍按 newAppWithFactories 中写死的顺序手工组装。这个内部结构只让
// 单元测试能够替换会连接 PostgreSQL、S3 或模型平台的构造步骤，从而验证创建顺序和失败清理。
type runtimeFactories struct {
	// newLogger 创建进程共享 slog.Logger。
	newLogger func(config.Log) (*slog.Logger, error)
	// openDatabase 建立并验证 PostgreSQL 连接池。
	openDatabase func(context.Context, config.Database) (*sqlx.DB, error)
	// closeDatabase 释放连接池，单独保存便于测试关闭失败。
	closeDatabase func(*sqlx.DB) error
	// newRepository 用连接池创建文档仓储接口实现。
	newRepository func(*sqlx.DB) (application.DocumentRepository, error)
	// newStorage 根据配置创建 S3 或本地对象存储。
	newStorage func(context.Context, config.Storage) (application.ObjectStorage, error)
	// newEmbedder 创建外部向量模型客户端。
	newEmbedder func(config.Embedding) (application.Embedder, error)
	// newService 把三个应用端口和策略组装成知识服务。
	newService func(application.Dependencies) (httpadapter.KnowledgeService, error)
	// newHandler 把应用服务注入 Gin HTTP Handler。
	newHandler func(httpadapter.KnowledgeService, *slog.Logger) (*httpadapter.Handler, error)
	// newMiddleware 创建请求 ID、访问日志和 Recovery 中间件。
	newMiddleware func(*slog.Logger) (httpserver.Middleware, error)
	// newRouter 注册 Gin 路由并返回标准库 http.Handler。
	newRouter func(*httpadapter.Handler, httpserver.Middleware) http.Handler
	// newServer 创建管理监听和优雅关闭的 Server。
	newServer func(config.HTTP, http.Handler, *slog.Logger) (serverRunner, error)
}

// App 保存服务运行期间真正需要长期持有的进程级资源。
//
// Handler、Repository 等对象已经通过引用链被 Server 持有，不需要在这里重复保存。数据库连接池
// 必须显式保留，因为它需要在 HTTP 服务退出后关闭。
type App struct {
	// server 持有完整 HTTP Handler 引用链并管理服务生命周期。
	server serverRunner
	// database 是 App 自己拥有并需要在退出时关闭的连接池。
	database *sqlx.DB
	// closeDatabase 抽象关闭动作，便于测试注入失败。
	closeDatabase func(*sqlx.DB) error
	// logger 在完整组装后被设置为进程默认日志器。
	logger *slog.Logger

	// Close 可能被正常退出路径和错误兜底路径同时调用。sync.Once 保证连接池最多关闭一次。
	closeOnce sync.Once
	closeErr  error
}

// Run 是 cmd/travel-agent 调用的进程级入口。
//
// 固定顺序是：读取环境变量、集中校验并组装全部依赖、设置默认结构化日志器，最后进入 HTTP
// 生命周期。任何一步失败都会把带上下文的错误交给 main 决定退出码。
func Run(ctx context.Context) error {
	// 生产入口始终从真实进程环境生成一次不可变配置快照。
	configuration, err := config.Load(os.Getenv)
	// 类型解析失败时尚未建立任何外部连接。
	if err != nil {
		return fmt.Errorf("load application configuration: %w", err)
	}

	// New 会完成集中校验和全部具体依赖组装，但还不会阻塞监听。
	runtime, err := New(ctx, configuration)
	// 构造失败已经负责清理可能打开的数据库连接。
	if err != nil {
		return err
	}

	// 只有完整组装成功后才替换默认日志器，避免半初始化状态影响同进程中的其他代码或测试。
	slog.SetDefault(runtime.logger)
	// 进入 App 生命周期，直到服务退出后关闭数据库并返回最终错误。
	return runtime.Run(ctx)
}

// New 使用生产构造函数建立一套完整 App，但暂不开始监听端口。
//
// 把“创建”和“运行”分开后，调用方可以先拿到完整对象；如果后续增加启动探针，也不会被迫把
// 所有逻辑塞进 main。当前配置会在任何外部连接建立前完成校验。
func New(ctx context.Context, configuration config.Config) (*App, error) {
	// 生产构造函数集合与可测试组装算法分离，运行对象图仍按固定代码顺序创建。
	return newAppWithFactories(ctx, configuration, defaultRuntimeFactories())
}

// Run 启动 HTTP 服务，并确保服务无论正常返回还是报错都关闭数据库连接池。
func (app *App) Run(ctx context.Context) error {
	// nil App 或缺失 Server 表示构造合同被绕过，不能进入运行阶段。
	if app == nil || app.server == nil {
		return fmt.Errorf("application server is required")
	}

	// Server.Run 阻塞到监听错误或 Context 取消。
	runErr := app.server.Run(ctx)
	// 无论运行结果如何，都立刻尝试释放 App 拥有的连接池。
	closeErr := app.Close()

	// errors.Join 会保留两个错误链：既不吞掉真正的运行错误，也不会隐藏连接池关闭失败。
	// errors.Is/errors.As 仍能沿链识别原始错误，调用方不需要依赖错误字符串。
	switch {
	case runErr != nil && closeErr != nil:
		// 两个错误同时存在时使用 errors.Join 完整保留。
		return errors.Join(
			fmt.Errorf("run HTTP server: %w", runErr),
			fmt.Errorf("close application resources: %w", closeErr),
		)
	case runErr != nil:
		// 只有运行错误时增加 HTTP 生命周期上下文。
		return fmt.Errorf("run HTTP server: %w", runErr)
	case closeErr != nil:
		// 服务正常结束但资源释放失败时仍应返回错误退出。
		return fmt.Errorf("close application resources: %w", closeErr)
	default:
		// 运行和关闭都成功时返回 nil。
		return nil
	}
}

// Close 释放 App 自己拥有的数据库连接池，并且可以安全地重复调用。
func (app *App) Close() error {
	// nil 接收者按已经关闭处理，方便错误兜底路径安全调用。
	if app == nil {
		return nil
	}

	// sync.Once 保证并发或重复调用最多执行一次真实关闭动作。
	app.closeOnce.Do(func() {
		// 测试对象或构造未完成对象可能没有数据库，直接跳过。
		if app.database == nil || app.closeDatabase == nil {
			return
		}
		// 保存首次关闭结果，后续调用返回同一个错误。
		app.closeErr = app.closeDatabase(app.database)
	})
	// 返回缓存结果，不会再次触发连接池关闭。
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

	// 第一个实际对象是日志器，后续构造错误可以使用它记录，但还不设置成全局默认值。
	logger, err := factories.newLogger(configuration.Log)
	if err != nil {
		return nil, fmt.Errorf("create application logger: %w", err)
	}
	// 自定义工厂返回 nil 但无错误仍属于构造失败。
	if logger == nil {
		return nil, fmt.Errorf("create application logger: constructor returned nil")
	}

	// 数据库是第一个需要显式关闭的外部资源。
	databaseConnection, err := factories.openDatabase(ctx, configuration.Database)
	if err != nil {
		return nil, fmt.Errorf("open application database: %w", err)
	}
	// 防御错误工厂返回 nil 连接池，避免 Repository 构造时才 panic。
	if databaseConnection == nil {
		return nil, fmt.Errorf("open application database: constructor returned nil")
	}

	// cleanupConstructionError 是数据库建立后的统一失败出口。清理失败会作为第二条错误链加入，
	// 但最先发生的构造错误仍用 %w 保留，errors.Is 仍然能准确识别它。
	cleanupConstructionError := func(step string, constructionErr error) error {
		// 主构造错误用 %w 包装，调用方仍可 errors.Is/As 识别。
		primaryErr := fmt.Errorf("%s: %w", step, constructionErr)
		// 关闭函数缺失本身也是资源管理错误，与主错误一起返回。
		if factories.closeDatabase == nil {
			return errors.Join(primaryErr, fmt.Errorf("close database after construction failure: close function is nil"))
		}
		// 关闭连接池失败时追加第二条错误链，不能覆盖最初失败步骤。
		if closeErr := factories.closeDatabase(databaseConnection); closeErr != nil {
			return errors.Join(
				primaryErr,
				fmt.Errorf("close database after construction failure: %w", closeErr),
			)
		}
		// 清理成功时只返回主构造错误。
		return primaryErr
	}

	// Repository 依赖已经打开的数据库连接池。
	repository, err := factories.newRepository(databaseConnection)
	if err != nil {
		return nil, cleanupConstructionError("create document repository", err)
	}

	// Storage 根据配置选择 S3/RustFS 或本地文件系统实现。
	objectStorage, err := factories.newStorage(ctx, configuration.Storage)
	if err != nil {
		return nil, cleanupConstructionError("create object storage", err)
	}

	// Embedder 使用模型地址、密钥、维度和超时构造。
	embedder, err := factories.newEmbedder(configuration.Embedding)
	if err != nil {
		return nil, cleanupConstructionError("create embedding client", err)
	}

	// 应用服务只接收小接口和业务策略，不认识任何具体实现类型。
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

	// Handler 只接收应用用例接口和日志器。
	handler, err := factories.newHandler(service, logger)
	if err != nil {
		return nil, cleanupConstructionError("create knowledge HTTP handler", err)
	}

	// 通用中间件与业务 Handler 分开构造。
	middleware, err := factories.newMiddleware(logger)
	if err != nil {
		return nil, cleanupConstructionError("create HTTP middleware", err)
	}

	// Router 本身没有需要关闭的资源，它只是把已经构造好的 Handler 和中间件注册到路由树。
	router := factories.newRouter(handler, middleware)
	// 标准库 Server 持有 Gin Engine，并配置全部超时。
	server, err := factories.newServer(configuration.HTTP, router, logger)
	if err != nil {
		return nil, cleanupConstructionError("create HTTP server", err)
	}

	// 所有依赖完整构造后才返回 App；此后连接池由 App.Close 负责。
	return &App{
		server:        server,
		database:      databaseConnection,
		closeDatabase: factories.closeDatabase,
		logger:        logger,
	}, nil
}

// defaultRuntimeFactories 把每个生产构造函数适配成组合根需要的统一签名。
func defaultRuntimeFactories() runtimeFactories {
	// 每个字段都明确绑定到一个生产构造函数，运行时没有按字符串解析依赖的容器。
	return runtimeFactories{
		newLogger:     newLogger,
		openDatabase:  database.Open,
		closeDatabase: func(database *sqlx.DB) error { return database.Close() },
		newRepository: func(database *sqlx.DB) (application.DocumentRepository, error) {
			// 具体 PostgreSQL 类型在这里向内收窄成应用仓储接口。
			return postgresadapter.NewRepository(database)
		},
		newStorage: func(ctx context.Context, configuration config.Storage) (application.ObjectStorage, error) {
			// 只有组合根知道并选择具体存储模式。
			if configuration.S3Enabled {
				return platformstorage.NewS3Storage(ctx, configuration)
			}
			return platformstorage.NewLocalStorage(configuration.LocalDir)
		},
		newEmbedder: func(configuration config.Embedding) (application.Embedder, error) {
			// 具体客户端向内收窄为 Embedder 接口。
			return embedding.NewClient(configuration)
		},
		newService: func(dependencies application.Dependencies) (httpadapter.KnowledgeService, error) {
			// *application.Service 同时满足 HTTP Adapter 需要的 KnowledgeService 接口。
			return application.NewService(dependencies)
		},
		newHandler:    httpadapter.NewHandler,
		newMiddleware: httpserver.NewMiddleware,
		newRouter: func(handler *httpadapter.Handler, middleware httpserver.Middleware) http.Handler {
			// *gin.Engine 实现标准库 http.Handler，外层 Server 无需依赖 Gin 类型。
			return httpadapter.NewRouter(handler, middleware)
		},
		newServer: func(configuration config.HTTP, handler http.Handler, logger *slog.Logger) (serverRunner, error) {
			// *Server 向内收窄成只包含 Run 的 serverRunner。
			return NewServer(configuration, handler, logger)
		},
	}
}

// newLogger 根据配置创建标准库 slog 日志器。
// JSON 适合生产日志采集，text 便于本地阅读；两种格式共享同一个等级过滤规则。
func newLogger(configuration config.Log) (*slog.Logger, error) {
	// level 保存解析后的 slog 强类型等级。
	var level slog.Level
	// 配置字符串只允许四种明确值。
	switch configuration.Level {
	case "debug":
		// debug 输出最详细开发信息。
		level = slog.LevelDebug
	case "info":
		// info 是默认生产等级。
		level = slog.LevelInfo
	case "warn":
		// warn 只输出警告和错误。
		level = slog.LevelWarn
	case "error":
		// error 只输出错误。
		level = slog.LevelError
	default:
		return nil, fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, error")
	}

	// HandlerOptions 把同一等级过滤规则交给不同格式 Handler。
	options := &slog.HandlerOptions{Level: level}
	// 接口变量允许根据配置保存 JSONHandler 或 TextHandler。
	var handler slog.Handler
	// 输出目标统一为 stdout，格式由配置选择。
	switch configuration.Format {
	case "json":
		// JSON 便于日志平台解析字段。
		handler = slog.NewJSONHandler(os.Stdout, options)
	case "text":
		// text 便于本地终端直接阅读。
		handler = slog.NewTextHandler(os.Stdout, options)
	default:
		return nil, fmt.Errorf("LOG_FORMAT must be json or text")
	}
	// 用已选 Handler 构造不可变 slog.Logger。
	return slog.New(handler), nil
}
