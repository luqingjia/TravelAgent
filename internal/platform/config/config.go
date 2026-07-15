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
	defaultPort                    = "8081"
	defaultReadHeaderTimeout       = 5 * time.Second
	defaultReadTimeout             = 60 * time.Second
	defaultWriteTimeout            = 5 * time.Minute
	defaultIdleTimeout             = 60 * time.Second
	defaultShutdownTimeout         = 15 * time.Second
	defaultDatabaseMaxOpenConns    = 10
	defaultDatabaseMaxIdleConns    = 5
	defaultDatabaseConnMaxLifetime = 30 * time.Minute
	defaultDatabaseConnMaxIdleTime = 5 * time.Minute
	defaultStorageEndpoint         = "http://localhost:9000"
	defaultStorageRegion           = "us-east-1"
	defaultLocalStorageDir         = ".data/storage"
	defaultEmbeddingBaseURL        = "https://dashscope.aliyuncs.com/compatible-mode"
	defaultEmbeddingModel          = "text-embedding-v3"
	defaultEmbeddingTimeout        = 60 * time.Second
	defaultAllowedExtensions       = "pdf,doc,docx,txt,md,markdown,html,htm"
	defaultMaximumUploadSize       = "50MB"
	defaultLogLevel                = "info"
	defaultLogFormat               = "json"
)

// Config 是服务启动所需的完整配置快照。
// 各子结构按运行职责分组，避免 app.New 接收几十个散落字符串，也避免业务包自己读取环境变量。
type Config struct {
	HTTP      HTTP
	Database  Database
	Storage   Storage
	Embedding Embedding
	Document  Document
	Log       Log
}

// HTTP 保存监听地址相关值和 http.Server 生命周期超时。
type HTTP struct {
	Port              string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

// Database 保存 PostgreSQL DSN 与 database/sql 连接池参数。
// DSN 只交给驱动使用，日志层不得原样输出它，因为其中通常包含用户名和密码。
type Database struct {
	DSN                string
	MaxOpenConnections int
	MaxIdleConnections int
	ConnMaxLifetime    time.Duration
	ConnMaxIdleTime    time.Duration
}

// Storage 描述 S3/RustFS 和本地文件系统两种互斥存储模式。
type Storage struct {
	S3Enabled    bool
	Bucket       string
	Endpoint     string
	Region       string
	AccessKey    string
	SecretKey    string
	UsePathStyle bool
	LocalDir     string
}

// Embedding 保存兼容 OpenAI embeddings 协议的客户端配置。
type Embedding struct {
	APIKey     string
	BaseURL    string
	Model      string
	Dimensions int
	Timeout    time.Duration
}

// Document 保存上传入口真正需要的文件策略。
type Document struct {
	AllowedExtensions map[string]struct{}
	MaxUploadBytes    int64
}

// Log 保存标准库 slog 的等级和输出格式。
type Log struct {
	Level  string
	Format string
}

// Load 使用调用方提供的 getenv 读取配置并解析类型。
// 生产环境传 os.Getenv；测试传 map-backed 函数，因此不需要修改进程级环境变量。
func Load(getenv func(string) string) (Config, error) {
	if getenv == nil {
		getenv = os.Getenv
	}

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

	s3Enabled, err := boolFromEnv(getenv, "RUSTFS_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	usePathStyle, err := boolFromEnv(getenv, "RUSTFS_PATH_STYLE", true)
	if err != nil {
		return Config{}, err
	}

	dimensions, err := intFromEnv(getenv, "EMBEDDING_DIMENSIONS", application.EmbeddingDimensions)
	if err != nil {
		return Config{}, err
	}
	embeddingTimeout, err := durationFromEnv(getenv, "EMBEDDING_TIMEOUT", defaultEmbeddingTimeout)
	if err != nil {
		return Config{}, err
	}
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
	port, err := strconv.Atoi(strings.TrimSpace(configuration.HTTP.Port))
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("GO_AGENT_PORT must be an integer between 1 and 65535")
	}
	if err := validatePositiveDurations(map[string]time.Duration{
		"HTTP_READ_HEADER_TIMEOUT": configuration.HTTP.ReadHeaderTimeout,
		"HTTP_READ_TIMEOUT":        configuration.HTTP.ReadTimeout,
		"HTTP_WRITE_TIMEOUT":       configuration.HTTP.WriteTimeout,
		"HTTP_IDLE_TIMEOUT":        configuration.HTTP.IdleTimeout,
		"HTTP_SHUTDOWN_TIMEOUT":    configuration.HTTP.ShutdownTimeout,
	}); err != nil {
		return err
	}

	if strings.TrimSpace(configuration.Database.DSN) == "" {
		return fmt.Errorf("POSTGRESQL_DSN is required")
	}
	if configuration.Database.MaxOpenConnections <= 0 {
		return fmt.Errorf("POSTGRESQL_MAX_OPEN_CONNS must be positive")
	}
	if configuration.Database.MaxIdleConnections < 0 ||
		configuration.Database.MaxIdleConnections > configuration.Database.MaxOpenConnections {
		return fmt.Errorf("POSTGRESQL_MAX_IDLE_CONNS must be between 0 and POSTGRESQL_MAX_OPEN_CONNS")
	}
	if configuration.Database.ConnMaxLifetime <= 0 {
		return fmt.Errorf("POSTGRESQL_CONN_MAX_LIFETIME must be positive")
	}
	if configuration.Database.ConnMaxIdleTime <= 0 {
		return fmt.Errorf("POSTGRESQL_CONN_MAX_IDLE_TIME must be positive")
	}

	if configuration.Storage.S3Enabled {
		if strings.TrimSpace(configuration.Storage.Bucket) == "" {
			return fmt.Errorf("RUSTFS_BUCKET_NAME is required when RUSTFS_ENABLED=true")
		}
		if strings.TrimSpace(configuration.Storage.AccessKey) == "" {
			return fmt.Errorf("RUSTFS_ACCESS_KEY is required when RUSTFS_ENABLED=true")
		}
		if strings.TrimSpace(configuration.Storage.SecretKey) == "" {
			return fmt.Errorf("RUSTFS_SECRET_KEY is required when RUSTFS_ENABLED=true")
		}
		if strings.TrimSpace(configuration.Storage.Region) == "" {
			return fmt.Errorf("RUSTFS_REGION is required when RUSTFS_ENABLED=true")
		}
		if err := validateHTTPURL("RUSTFS_ENDPOINT", configuration.Storage.Endpoint); err != nil {
			return err
		}
	} else if strings.TrimSpace(configuration.Storage.LocalDir) == "" {
		return fmt.Errorf("LOCAL_STORAGE_DIR is required when RUSTFS_ENABLED=false")
	}

	if strings.TrimSpace(configuration.Embedding.APIKey) == "" {
		return fmt.Errorf("EMBEDDING_API_KEY is required")
	}
	if err := validateHTTPURL("EMBEDDING_BASE_URL", configuration.Embedding.BaseURL); err != nil {
		return err
	}
	if strings.TrimSpace(configuration.Embedding.Model) == "" {
		return fmt.Errorf("EMBEDDING_MODEL is required")
	}
	if configuration.Embedding.Dimensions != application.EmbeddingDimensions {
		return fmt.Errorf(
			"EMBEDDING_DIMENSIONS must be %d to match database vector(%d)",
			application.EmbeddingDimensions,
			application.EmbeddingDimensions,
		)
	}
	if configuration.Embedding.Timeout <= 0 {
		return fmt.Errorf("EMBEDDING_TIMEOUT must be positive")
	}

	if configuration.Document.MaxUploadBytes <= 0 {
		return fmt.Errorf("KNOWLEDGE_DOCUMENT_MAX_SIZE must be positive")
	}
	if len(configuration.Document.AllowedExtensions) == 0 {
		return fmt.Errorf("KNOWLEDGE_DOCUMENT_ALLOWED_EXTENSIONS must contain at least one extension")
	}

	switch configuration.Log.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, error")
	}
	switch configuration.Log.Format {
	case "json", "text":
	default:
		return fmt.Errorf("LOG_FORMAT must be json or text")
	}

	return nil
}

// IsExtensionAllowed 统一扩展名大小写和开头点号后检查上传白名单。
func (document Document) IsExtensionAllowed(extension string) bool {
	normalized := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(extension), "."))
	if normalized == "" {
		return false
	}
	_, allowed := document.AllowedExtensions[normalized]
	return allowed
}

// valueOrDefault 读取去除首尾空白后的值；空值使用明确默认值。
func valueOrDefault(getenv func(string) string, key string, fallback string) string {
	value := strings.TrimSpace(getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

// durationFromEnv 使用 time.ParseDuration 解析 5s、2m 等 Go 标准时长写法，并在错误里保留变量名。
func durationFromEnv(getenv func(string) string, key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration: %w", key, err)
	}
	return parsed, nil
}

// intFromEnv 解析十进制整数；范围和跨字段关系由 Validate 统一检查。
func intFromEnv(getenv func(string) string, key string, fallback int) (int, error) {
	value := strings.TrimSpace(getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return parsed, nil
}

// boolFromEnv 接受常见真假写法，但会拒绝拼错的值，避免 `ture` 被静默当成 false。
func boolFromEnv(getenv func(string) string, key string, fallback bool) (bool, error) {
	value := strings.ToLower(strings.TrimSpace(getenv(key)))
	if value == "" {
		return fallback, nil
	}
	switch value {
	case "true", "1", "yes", "y":
		return true, nil
	case "false", "0", "no", "n":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be a boolean", key)
	}
}

// parseExtensions 把逗号分隔扩展名整理成小写、无点号的集合，并自动去重。
func parseExtensions(value string) map[string]struct{} {
	extensions := make(map[string]struct{})
	for _, item := range strings.Split(value, ",") {
		extension := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(item), "."))
		if extension != "" {
			extensions[extension] = struct{}{}
		}
	}
	return extensions
}

// parseSizeBytes 把 50MB、2KB 或纯字节数转换成 int64，并防止负数和乘法溢出。
func parseSizeBytes(value string) (int64, error) {
	raw := strings.ToUpper(strings.TrimSpace(value))
	if raw == "" {
		return 0, fmt.Errorf("size cannot be empty")
	}

	multiplier := int64(1)
	for _, suffix := range []struct {
		text       string
		multiplier int64
	}{
		{text: "GB", multiplier: 1024 * 1024 * 1024},
		{text: "MB", multiplier: 1024 * 1024},
		{text: "KB", multiplier: 1024},
		{text: "B", multiplier: 1},
	} {
		if strings.HasSuffix(raw, suffix.text) {
			multiplier = suffix.multiplier
			raw = strings.TrimSpace(strings.TrimSuffix(raw, suffix.text))
			break
		}
	}

	amount, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", value, err)
	}
	if amount < 0 {
		return 0, fmt.Errorf("size cannot be negative")
	}
	if amount > math.MaxInt64/multiplier {
		return 0, fmt.Errorf("size %q overflows int64 bytes", value)
	}
	return amount * multiplier, nil
}

// validatePositiveDurations 对 HTTP 超时逐项检查，并返回可直接定位到环境变量的错误。
func validatePositiveDurations(values map[string]time.Duration) error {
	for key, value := range values {
		if value <= 0 {
			return fmt.Errorf("%s must be positive", key)
		}
	}
	return nil
}

// validateHTTPURL 确保需要发网络请求的地址包含 http/https 协议和主机名。
func validateHTTPURL(key string, value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%s must be an absolute http or https URL", key)
	}
	return nil
}
