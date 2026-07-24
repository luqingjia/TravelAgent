// Package config 集中读取并校验 TravelAgent 进程需要的环境变量。
//
// Load 只负责把字符串解析成有类型的配置，Validate 负责检查跨字段业务约束。把两步拆开后，
// 测试可以精确判断是“格式写错”还是“必要配置缺失”，组合根也能在建立任何外部连接前一次失败。
package config

import (
	"fmt"
	"math"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/luqingjia/TravelAgent/internal/knowledge/application"
)

const (
	// defaultPort 是未设置 GO_AGENT_PORT 时使用的 HTTP 端口。
	defaultPort = "8081"
	// defaultReadHeaderTimeout 限制读取请求头时间，降低慢速请求占用连接的风险。
	defaultReadHeaderTimeout = 5 * time.Second
	// defaultReadTimeout 限制读取完整请求的时间。
	defaultReadTimeout = 60 * time.Second
	// defaultWriteTimeout 为同步文档处理响应保留较长写出窗口。
	defaultWriteTimeout = 5 * time.Minute
	// defaultIdleTimeout 控制 Keep-Alive 空闲连接保留时间。
	defaultIdleTimeout = 60 * time.Second
	// defaultShutdownTimeout 是优雅关闭等待在途请求的最长时间。
	defaultShutdownTimeout = 15 * time.Second
	// defaultDatabaseMaxOpenConns 限制数据库连接池总连接数。
	defaultDatabaseMaxOpenConns = 10
	// defaultDatabaseMaxIdleConns 限制连接池保留的空闲连接数。
	defaultDatabaseMaxIdleConns = 5
	// defaultDatabaseConnMaxLifetime 控制单个连接最长复用时间。
	defaultDatabaseConnMaxLifetime = 30 * time.Minute
	// defaultDatabaseConnMaxIdleTime 控制空闲连接最长停留时间。
	defaultDatabaseConnMaxIdleTime = 5 * time.Minute
	// defaultStorageEndpoint 是本地 RustFS 常用地址。
	defaultStorageEndpoint = "http://localhost:9000"
	// defaultStorageRegion 是 S3 兼容客户端需要的默认区域。
	defaultStorageRegion = "us-east-1"
	// defaultLocalStorageDir 是关闭 S3 时保存上传文件的目录。
	defaultLocalStorageDir = ".data/storage"
	// defaultEmbeddingBaseURL 是当前 OpenAI 兼容 Embedding 服务地址。
	defaultEmbeddingBaseURL = "https://dashscope.aliyuncs.com/compatible-mode"
	// defaultEmbeddingModel 是当前默认向量模型专有名称。
	defaultEmbeddingModel = "text-embedding-v3"
	// defaultEmbeddingTimeout 限制单次模型 HTTP 请求时间。
	defaultEmbeddingTimeout = 60 * time.Second
	// defaultAllowedExtensions 列出当前上传入口接受的文件扩展名。
	defaultAllowedExtensions = "pdf,doc,docx,txt,md,markdown,html,htm"
	// defaultMaximumUploadSize 是默认 50 MiB 上传上限的可读配置写法。
	defaultMaximumUploadSize = "50MB"
	// defaultLogLevel 控制默认只输出 info 及以上日志。
	defaultLogLevel = "info"
	// defaultLogFormat 默认使用适合日志平台采集的 JSON。
	defaultLogFormat = "json"
)

// Config 是服务启动所需的完整配置快照。
// 各子结构按运行职责分组，避免 app.New 接收几十个散落字符串，也避免业务包自己读取环境变量。
type Config struct {
	// HTTP 保存监听和生命周期参数。
	HTTP HTTP
	// Database 保存 PostgreSQL 连接和连接池参数。
	Database Database
	// Storage 保存对象存储模式和凭据。
	Storage Storage
	// Embedding 保存向量模型客户端参数。
	Embedding Embedding
	// Document 保存上传文件策略。
	Document Document
	// Log 保存 slog 输出策略。
	Log Log
}

// HTTP 保存监听地址相关值和 http.Server 生命周期超时。
type HTTP struct {
	// Port 是监听端口字符串，最终会由 Server 构造器转换成整数。
	Port string
	// ReadHeaderTimeout 限制读取请求头时间。
	ReadHeaderTimeout time.Duration
	// ReadTimeout 限制读取完整请求时间。
	ReadTimeout time.Duration
	// WriteTimeout 限制响应写出时间。
	WriteTimeout time.Duration
	// IdleTimeout 限制空闲 Keep-Alive 连接时间。
	IdleTimeout time.Duration
	// ShutdownTimeout 限制优雅关闭等待时间。
	ShutdownTimeout time.Duration
}

// Database 保存 PostgreSQL DSN 与 database/sql 连接池参数。
// DSN 只交给驱动使用，日志层不得原样输出它，因为其中通常包含用户名和密码。
type Database struct {
	// DSN 是包含地址、库名和凭据的 PostgreSQL 连接串，禁止写入日志。
	DSN string
	// MaxOpenConnections 是连接池最大打开连接数。
	MaxOpenConnections int
	// MaxIdleConnections 是连接池最大空闲连接数。
	MaxIdleConnections int
	// ConnMaxLifetime 是单个连接最长生命周期。
	ConnMaxLifetime time.Duration
	// ConnMaxIdleTime 是单个连接最长空闲时间。
	ConnMaxIdleTime time.Duration
}

// Storage 描述 S3/RustFS 和本地文件系统两种互斥存储模式。
type Storage struct {
	// S3Enabled 决定组合根选择 S3Storage 还是 LocalStorage。
	S3Enabled bool
	// Bucket 是 S3/RustFS 桶名。
	Bucket string
	// Endpoint 是 S3 兼容服务绝对 URL。
	Endpoint string
	// Region 是 AWS SDK 签名使用的区域。
	Region string
	// AccessKey 是对象存储访问标识，禁止写入日志。
	AccessKey string
	// SecretKey 是对象存储密钥，禁止写入日志。
	SecretKey string
	// UsePathStyle 控制 S3 客户端是否使用路径风格地址。
	UsePathStyle bool
	// LocalDir 是本地存储模式的根目录。
	LocalDir string
}

// Embedding 保存兼容 OpenAI embeddings 协议的客户端配置。
type Embedding struct {
	// APIKey 是模型服务鉴权密钥，禁止写入日志。
	APIKey string
	// BaseURL 是 OpenAI 兼容服务基础地址。
	BaseURL string
	// Model 是请求使用的模型专有名称。
	Model string
	// Dimensions 必须固定为数据库 vector(1536) 对应值。
	Dimensions int
	// Timeout 是单次 Embedding 请求超时。
	Timeout time.Duration
}

// Document 保存上传入口真正需要的文件策略。
type Document struct {
	// AllowedExtensions 是小写、无点号扩展名集合。
	AllowedExtensions map[string]struct{}
	// MaxUploadBytes 是读取上传流时允许的最大真实字节数。
	MaxUploadBytes int64
}

// Log 保存标准库 slog 的等级和输出格式。
type Log struct {
	// Level 是 debug、info、warn、error 之一。
	Level string
	// Format 是 json 或 text。
	Format string
}

// Load 使用调用方提供的 getenv 读取配置并解析类型。
// 生产环境传 os.Getenv；测试传 map-backed 函数，因此不需要修改进程级环境变量。
func Load(getenv func(string) string) (Config, error) {
	// nil 表示生产调用方没有注入测试读取函数，回退到真实进程环境。
	if getenv == nil {
		getenv = os.Getenv
	}

	// 依次解析 HTTP 生命周期参数，任何格式错误都立即返回具体环境变量名。
	readHeaderTimeout, err := durationFromEnv(getenv, "HTTP_READ_HEADER_TIMEOUT", defaultReadHeaderTimeout)
	if err != nil {
		return Config{}, err
	}
	readTimeout, err := durationFromEnv(getenv, "HTTP_READ_TIMEOUT", defaultReadTimeout)
	if err != nil {
		return Config{}, err
	}
	writeTimeout, err := durationFromEnv(getenv, "HTTP_WRITE_TIMEOUT", defaultWriteTimeout)
	if err != nil {
		return Config{}, err
	}
	idleTimeout, err := durationFromEnv(getenv, "HTTP_IDLE_TIMEOUT", defaultIdleTimeout)
	if err != nil {
		return Config{}, err
	}
	shutdownTimeout, err := durationFromEnv(getenv, "HTTP_SHUTDOWN_TIMEOUT", defaultShutdownTimeout)
	if err != nil {
		return Config{}, err
	}

	// 数据库连接池整数和时长在建立连接前完成类型转换。
	maxOpenConnections, err := intFromEnv(getenv, "POSTGRESQL_MAX_OPEN_CONNS", defaultDatabaseMaxOpenConns)
	if err != nil {
		return Config{}, err
	}
	maxIdleConnections, err := intFromEnv(getenv, "POSTGRESQL_MAX_IDLE_CONNS", defaultDatabaseMaxIdleConns)
	if err != nil {
		return Config{}, err
	}
	connMaxLifetime, err := durationFromEnv(getenv, "POSTGRESQL_CONN_MAX_LIFETIME", defaultDatabaseConnMaxLifetime)
	if err != nil {
		return Config{}, err
	}
	connMaxIdleTime, err := durationFromEnv(getenv, "POSTGRESQL_CONN_MAX_IDLE_TIME", defaultDatabaseConnMaxIdleTime)
	if err != nil {
		return Config{}, err
	}

	// 存储模式开关接受常见布尔写法，但拒绝拼写错误。
	s3Enabled, err := boolFromEnv(getenv, "RUSTFS_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	usePathStyle, err := boolFromEnv(getenv, "RUSTFS_PATH_STYLE", true)
	if err != nil {
		return Config{}, err
	}

	// Embedding 维度默认直接引用应用合同，避免配置默认值与处理校验漂移。
	dimensions, err := intFromEnv(getenv, "EMBEDDING_DIMENSIONS", application.EmbeddingDimensions)
	if err != nil {
		return Config{}, err
	}
	embeddingTimeout, err := durationFromEnv(getenv, "EMBEDDING_TIMEOUT", defaultEmbeddingTimeout)
	if err != nil {
		return Config{}, err
	}
	// 上传大小允许 50MB 等可读写法，最终统一转换成 int64 字节数。
	maxUploadBytes, err := parseSizeBytes(valueOrDefault(getenv, "KNOWLEDGE_DOCUMENT_MAX_SIZE", defaultMaximumUploadSize))
	if err != nil {
		return Config{}, fmt.Errorf("KNOWLEDGE_DOCUMENT_MAX_SIZE: %w", err)
	}

	// 到这里所有字符串都已经成功转换为目标类型，但必要字段是否齐全由 Validate 统一判断。
	return Config{
		HTTP: HTTP{
			Port:              valueOrDefault(getenv, "GO_AGENT_PORT", defaultPort),
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
			ShutdownTimeout:   shutdownTimeout,
		},
		Database: Database{
			DSN:                strings.TrimSpace(getenv("POSTGRESQL_DSN")),
			MaxOpenConnections: maxOpenConnections,
			MaxIdleConnections: maxIdleConnections,
			ConnMaxLifetime:    connMaxLifetime,
			ConnMaxIdleTime:    connMaxIdleTime,
		},
		Storage: Storage{
			S3Enabled:    s3Enabled,
			Bucket:       strings.TrimSpace(getenv("RUSTFS_BUCKET_NAME")),
			Endpoint:     valueOrDefault(getenv, "RUSTFS_ENDPOINT", defaultStorageEndpoint),
			Region:       valueOrDefault(getenv, "RUSTFS_REGION", defaultStorageRegion),
			AccessKey:    strings.TrimSpace(getenv("RUSTFS_ACCESS_KEY")),
			SecretKey:    strings.TrimSpace(getenv("RUSTFS_SECRET_KEY")),
			UsePathStyle: usePathStyle,
			LocalDir:     valueOrDefault(getenv, "LOCAL_STORAGE_DIR", defaultLocalStorageDir),
		},
		Embedding: Embedding{
			APIKey:     strings.TrimSpace(getenv("EMBEDDING_API_KEY")),
			BaseURL:    valueOrDefault(getenv, "EMBEDDING_BASE_URL", defaultEmbeddingBaseURL),
			Model:      valueOrDefault(getenv, "EMBEDDING_MODEL", defaultEmbeddingModel),
			Dimensions: dimensions,
			Timeout:    embeddingTimeout,
		},
		Document: Document{
			AllowedExtensions: parseExtensions(valueOrDefault(getenv, "KNOWLEDGE_DOCUMENT_ALLOWED_EXTENSIONS", defaultAllowedExtensions)),
			MaxUploadBytes:    maxUploadBytes,
		},
		Log: Log{
			Level:  strings.ToLower(valueOrDefault(getenv, "LOG_LEVEL", defaultLogLevel)),
			Format: strings.ToLower(valueOrDefault(getenv, "LOG_FORMAT", defaultLogFormat)),
		},
	}, nil
}

// Validate 检查建立外部连接和启动监听之前必须满足的全部约束。
func (configuration Config) Validate() error {
	// 端口先转换成整数，并限制在 TCP 合法范围 1 到 65535。
	port, err := strconv.Atoi(strings.TrimSpace(configuration.HTTP.Port))
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("GO_AGENT_PORT must be an integer between 1 and 65535")
	}
	// 所有 HTTP 超时都必须为正数，零值会导致 Server 行为异常。
	if err := validatePositiveDurations(map[string]time.Duration{
		"HTTP_READ_HEADER_TIMEOUT": configuration.HTTP.ReadHeaderTimeout,
		"HTTP_READ_TIMEOUT":        configuration.HTTP.ReadTimeout,
		"HTTP_WRITE_TIMEOUT":       configuration.HTTP.WriteTimeout,
		"HTTP_IDLE_TIMEOUT":        configuration.HTTP.IdleTimeout,
		"HTTP_SHUTDOWN_TIMEOUT":    configuration.HTTP.ShutdownTimeout,
	}); err != nil {
		return err
	}

	// DSN 是数据库启动的必要条件，但错误消息只提变量名，不回显敏感值。
	if strings.TrimSpace(configuration.Database.DSN) == "" {
		return fmt.Errorf("POSTGRESQL_DSN is required")
	}
	// 最大连接数必须至少允许一个可用连接。
	if configuration.Database.MaxOpenConnections <= 0 {
		return fmt.Errorf("POSTGRESQL_MAX_OPEN_CONNS must be positive")
	}
	// 空闲连接数不能为负，也不能超过连接池总上限。
	if configuration.Database.MaxIdleConnections < 0 ||
		configuration.Database.MaxIdleConnections > configuration.Database.MaxOpenConnections {
		return fmt.Errorf("POSTGRESQL_MAX_IDLE_CONNS must be between 0 and POSTGRESQL_MAX_OPEN_CONNS")
	}
	// 连接最长生命周期必须为正数。
	if configuration.Database.ConnMaxLifetime <= 0 {
		return fmt.Errorf("POSTGRESQL_CONN_MAX_LIFETIME must be positive")
	}
	// 空闲回收时间必须为正数。
	if configuration.Database.ConnMaxIdleTime <= 0 {
		return fmt.Errorf("POSTGRESQL_CONN_MAX_IDLE_TIME must be positive")
	}

	// S3 模式需要完整桶、凭据、区域和端点；本地模式只要求目录。
	if configuration.Storage.S3Enabled {
		// 没有桶名就无法确定对象写入位置。
		if strings.TrimSpace(configuration.Storage.Bucket) == "" {
			return fmt.Errorf("RUSTFS_BUCKET_NAME is required when RUSTFS_ENABLED=true")
		}
		// AccessKey 缺失时 AWS SDK 无法签名请求。
		if strings.TrimSpace(configuration.Storage.AccessKey) == "" {
			return fmt.Errorf("RUSTFS_ACCESS_KEY is required when RUSTFS_ENABLED=true")
		}
		// SecretKey 缺失时同样无法完成鉴权。
		if strings.TrimSpace(configuration.Storage.SecretKey) == "" {
			return fmt.Errorf("RUSTFS_SECRET_KEY is required when RUSTFS_ENABLED=true")
		}
		// Region 会参与 AWS 签名，即使 RustFS 不做区域隔离也必须提供。
		if strings.TrimSpace(configuration.Storage.Region) == "" {
			return fmt.Errorf("RUSTFS_REGION is required when RUSTFS_ENABLED=true")
		}
		// Endpoint 必须是可发送 HTTP 请求的绝对地址。
		if err := validateHTTPURL("RUSTFS_ENDPOINT", configuration.Storage.Endpoint); err != nil {
			return err
		}
	} else if strings.TrimSpace(configuration.Storage.LocalDir) == "" {
		// 关闭 S3 后必须有本地根目录承载原始文件。
		return fmt.Errorf("LOCAL_STORAGE_DIR is required when RUSTFS_ENABLED=false")
	}

	// Embedding APIKey 是当前真实客户端的必要鉴权配置。
	if strings.TrimSpace(configuration.Embedding.APIKey) == "" {
		return fmt.Errorf("EMBEDDING_API_KEY is required")
	}
	// BaseURL 必须是 http/https 绝对地址。
	if err := validateHTTPURL("EMBEDDING_BASE_URL", configuration.Embedding.BaseURL); err != nil {
		return err
	}
	// 模型名称为空时无法构造协议兼容请求。
	if strings.TrimSpace(configuration.Embedding.Model) == "" {
		return fmt.Errorf("EMBEDDING_MODEL is required")
	}
	// 维度必须与应用层校验和数据库列严格一致。
	if configuration.Embedding.Dimensions != application.EmbeddingDimensions {
		return fmt.Errorf(
			"EMBEDDING_DIMENSIONS must be %d to match database vector(%d)",
			application.EmbeddingDimensions,
			application.EmbeddingDimensions,
		)
	}
	// 模型请求超时必须为正数。
	if configuration.Embedding.Timeout <= 0 {
		return fmt.Errorf("EMBEDDING_TIMEOUT must be positive")
	}

	// 上传上限必须为正数，零会让任何文件都无法通过。
	if configuration.Document.MaxUploadBytes <= 0 {
		return fmt.Errorf("KNOWLEDGE_DOCUMENT_MAX_SIZE must be positive")
	}
	// 扩展名集合不能为空，否则服务启动后所有上传都会被拒绝。
	if len(configuration.Document.AllowedExtensions) == 0 {
		return fmt.Errorf("KNOWLEDGE_DOCUMENT_ALLOWED_EXTENSIONS must contain at least one extension")
	}

	// slog 只接受项目明确支持的四个等级字符串。
	switch configuration.Log.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, error")
	}
	// 输出格式只允许结构化 JSON 或便于本地阅读的 text。
	switch configuration.Log.Format {
	case "json", "text":
	default:
		return fmt.Errorf("LOG_FORMAT must be json or text")
	}

	// 所有跨字段约束通过后，组合根才可以开始建立外部连接。
	return nil
}

// IsExtensionAllowed 统一扩展名大小写和开头点号后检查上传白名单。
func (document Document) IsExtensionAllowed(extension string) bool {
	// 文件名扩展统一成小写无点号形式，与集合键保持一致。
	normalized := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(extension), "."))
	// 空扩展名不代表任何允许类型。
	if normalized == "" {
		return false
	}
	// 利用 map 查询第二个返回值判断是否属于白名单。
	_, allowed := document.AllowedExtensions[normalized]
	// 返回结果供 HTTP 或配置测试直接使用。
	return allowed
}

// valueOrDefault 读取去除首尾空白后的值；空值使用明确默认值。
func valueOrDefault(getenv func(string) string, key string, fallback string) string {
	// 环境变量统一去首尾空格，避免只包含空格的值覆盖默认配置。
	value := strings.TrimSpace(getenv(key))
	if value == "" {
		return fallback
	}
	// 非空显式配置优先于默认值。
	return value
}

// durationFromEnv 使用 time.ParseDuration 解析 5s、2m 等 Go 标准时长写法，并在错误里保留变量名。
func durationFromEnv(getenv func(string) string, key string, fallback time.Duration) (time.Duration, error) {
	// 未设置时直接使用对应默认时长。
	value := strings.TrimSpace(getenv(key))
	if value == "" {
		return fallback, nil
	}
	// 标准库负责识别 ns、ms、s、m、h 等单位。
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration: %w", key, err)
	}
	// 返回强类型 time.Duration，后续不再处理原始字符串。
	return parsed, nil
}

// intFromEnv 解析十进制整数；范围和跨字段关系由 Validate 统一检查。
func intFromEnv(getenv func(string) string, key string, fallback int) (int, error) {
	// 空值使用调用方提供的整数默认值。
	value := strings.TrimSpace(getenv(key))
	if value == "" {
		return fallback, nil
	}
	// Atoi 按十进制解析，格式错误会带环境变量名返回。
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	// 数值范围等业务约束留给 Validate 统一判断。
	return parsed, nil
}

// boolFromEnv 接受常见真假写法，但会拒绝拼错的值，避免 `ture` 被静默当成 false。
func boolFromEnv(getenv func(string) string, key string, fallback bool) (bool, error) {
	// 布尔文本统一去空格并转小写。
	value := strings.ToLower(strings.TrimSpace(getenv(key)))
	if value == "" {
		return fallback, nil
	}
	// 接受部署配置中常见的几组真假写法。
	switch value {
	case "true", "1", "yes", "y":
		// 明确真值统一转换成 true。
		return true, nil
	case "false", "0", "no", "n":
		// 明确假值统一转换成 false。
		return false, nil
	default:
		// 任何其他拼写都返回错误，不能静默改变存储模式。
		return false, fmt.Errorf("%s must be a boolean", key)
	}
}

// parseExtensions 把逗号分隔扩展名整理成小写、无点号的集合，并自动去重。
func parseExtensions(value string) map[string]struct{} {
	// 使用 map 作为集合，重复扩展名会自然覆盖而不产生重复项。
	extensions := make(map[string]struct{})
	// 按逗号拆分配置中的每个候选扩展名。
	for _, item := range strings.Split(value, ",") {
		// 统一大小写、首尾空格和可选点号。
		extension := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(item), "."))
		// 空项直接忽略，例如配置末尾多写一个逗号。
		if extension != "" {
			extensions[extension] = struct{}{}
		}
	}
	// 返回可直接用于 O(1) 查询的扩展名集合。
	return extensions
}

// parseSizeBytes 把 50MB、2KB 或纯字节数转换成 int64，并防止负数和乘法溢出。
func parseSizeBytes(value string) (int64, error) {
	// 单位统一转大写，允许调用方写 mb、Mb 等形式。
	raw := strings.ToUpper(strings.TrimSpace(value))
	if raw == "" {
		return 0, fmt.Errorf("size cannot be empty")
	}

	// 默认乘数为 1，纯数字按字节解释。
	multiplier := int64(1)
	// 按最长单位优先匹配，防止 MB 先被 B 错误截断。
	for _, suffix := range []struct {
		// text 是配置后缀文本。
		text string
		// multiplier 是该单位对应的二进制字节倍数。
		multiplier int64
	}{
		{text: "GB", multiplier: 1024 * 1024 * 1024},
		{text: "MB", multiplier: 1024 * 1024},
		{text: "KB", multiplier: 1024},
		{text: "B", multiplier: 1},
	} {
		// 找到后缀后记录倍数，并从数值文本中移除单位。
		if strings.HasSuffix(raw, suffix.text) {
			multiplier = suffix.multiplier
			raw = strings.TrimSpace(strings.TrimSuffix(raw, suffix.text))
			// 单位已经唯一确定，无需继续尝试更短后缀。
			break
		}
	}

	// 剩余文本必须是十进制 int64。
	amount, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", value, err)
	}
	// 负上传大小没有业务意义。
	if amount < 0 {
		return 0, fmt.Errorf("size cannot be negative")
	}
	// 乘法前检查溢出，防止大配置绕回负数或小数。
	if amount > math.MaxInt64/multiplier {
		return 0, fmt.Errorf("size %q overflows int64 bytes", value)
	}
	// 安全相乘后得到统一字节数。
	return amount * multiplier, nil
}

// validatePositiveDurations 对 HTTP 超时逐项检查，并返回可直接定位到环境变量的错误。
func validatePositiveDurations(values map[string]time.Duration) error {
	// 逐项保留环境变量名，首个非法值即可给出准确错误。
	for key, value := range values {
		// 零和负时长都会让 Server 超时语义失效。
		if value <= 0 {
			return fmt.Errorf("%s must be positive", key)
		}
	}
	// 全部时长为正时返回 nil。
	return nil
}

// validateHTTPURL 确保需要发网络请求的地址包含 http/https 协议和主机名。
func validateHTTPURL(key string, value string) error {
	// Parse 负责拆分 scheme、host 和路径，但不会自动要求绝对 URL。
	parsed, err := url.Parse(strings.TrimSpace(value))
	// 同时检查解析错误、主机名和允许协议，拒绝相对地址或其他协议。
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%s must be an absolute http or https URL", key)
	}
	// 合法地址可以安全交给 HTTP 或 AWS SDK 客户端。
	return nil
}
