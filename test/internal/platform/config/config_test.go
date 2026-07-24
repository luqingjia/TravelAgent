package config_test

import (
	. "github.com/luqingjia/TravelAgent/internal/platform/config"
	"strings"
	"testing"
	"time"

	"github.com/luqingjia/TravelAgent/internal/knowledge/application"
)

// TestLoadUsesProductionSafeDefaults 验证没有可选环境变量时，HTTP 生命周期、日志、向量和上传策略使用设计约定的默认值。
func TestLoadUsesProductionSafeDefaults(t *testing.T) {
	// 准备：传入一个永远返回空字符串的 getenv，测试不会读写真实进程环境，因此可以稳定并行执行。
	configuration, err := Load(mapGetenv(nil))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// 关键断言：这些值直接影响接口兼容性、慢请求是否被误杀以及关停时是否有收尾时间。
	if configuration.HTTP.Port != "8081" {
		t.Fatalf("HTTP.Port = %q, want 8081", configuration.HTTP.Port)
	}
	if configuration.HTTP.ReadHeaderTimeout != 5*time.Second ||
		configuration.HTTP.ReadTimeout != 60*time.Second ||
		configuration.HTTP.WriteTimeout != 5*time.Minute ||
		configuration.HTTP.IdleTimeout != 60*time.Second ||
		configuration.HTTP.ShutdownTimeout != 15*time.Second {
		t.Fatalf("HTTP 默认超时不正确：%#v", configuration.HTTP)
	}
	if configuration.Log.Level != "info" || configuration.Log.Format != "json" {
		t.Fatalf("Log defaults = %#v", configuration.Log)
	}
	if configuration.Embedding.Dimensions != application.EmbeddingDimensions {
		t.Fatalf("Embedding.Dimensions = %d, want %d", configuration.Embedding.Dimensions, application.EmbeddingDimensions)
	}
	if configuration.Document.MaxUploadBytes != 50*1024*1024 {
		t.Fatalf("Document.MaxUploadBytes = %d, want 50MiB", configuration.Document.MaxUploadBytes)
	}
	if !configuration.Document.IsExtensionAllowed(".md") || !configuration.Document.IsExtensionAllowed("markdown") {
		t.Fatal("默认扩展名应包含 md 和 markdown")
	}
	if configuration.Database.MaxOpenConnections != 10 || configuration.Database.MaxIdleConnections != 5 {
		t.Fatalf("Database pool defaults = %#v", configuration.Database)
	}
}

// TestLoadParsesEnvironmentOverrides 验证字符串、整数、布尔值、容量和 duration 都能从统一入口正确转换。
func TestLoadParsesEnvironmentOverrides(t *testing.T) {
	// values 同时覆盖 HTTP、数据库、存储、Embedding、上传策略和日志，验证统一读取入口。
	values := map[string]string{
		"GO_AGENT_PORT":                         "9090",
		"POSTGRESQL_DSN":                        "postgres://example",
		"POSTGRESQL_MAX_OPEN_CONNS":             "24",
		"POSTGRESQL_MAX_IDLE_CONNS":             "12",
		"POSTGRESQL_CONN_MAX_LIFETIME":          "45m",
		"POSTGRESQL_CONN_MAX_IDLE_TIME":         "7m",
		"HTTP_READ_HEADER_TIMEOUT":              "3s",
		"HTTP_READ_TIMEOUT":                     "30s",
		"HTTP_WRITE_TIMEOUT":                    "2m",
		"HTTP_IDLE_TIMEOUT":                     "40s",
		"HTTP_SHUTDOWN_TIMEOUT":                 "9s",
		"RUSTFS_ENABLED":                        "false",
		"LOCAL_STORAGE_DIR":                     "tmp/files",
		"EMBEDDING_API_KEY":                     "test-key",
		"EMBEDDING_TIMEOUT":                     "20s",
		"KNOWLEDGE_DOCUMENT_ALLOWED_EXTENSIONS": "txt, .MD",
		"KNOWLEDGE_DOCUMENT_MAX_SIZE":           "2KB",
		"LOG_LEVEL":                             "debug",
		"LOG_FORMAT":                            "text",
	}

	// mapGetenv 代替 os.Getenv，整个测试不会依赖开发机当前环境变量。
	configuration, err := Load(mapGetenv(values))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	// HTTP 字符串和 duration 都必须按目标类型转换。
	if configuration.HTTP.Port != "9090" || configuration.HTTP.WriteTimeout != 2*time.Minute {
		t.Fatalf("HTTP overrides = %#v", configuration.HTTP)
	}
	// 数据库连接数量和生命周期影响连接池资源，四项覆盖值都要准确生效。
	if configuration.Database.MaxOpenConnections != 24 ||
		configuration.Database.MaxIdleConnections != 12 ||
		configuration.Database.ConnMaxLifetime != 45*time.Minute ||
		configuration.Database.ConnMaxIdleTime != 7*time.Minute {
		t.Fatalf("Database overrides = %#v", configuration.Database)
	}
	// 关闭 S3 后应切换到明确指定的本地目录。
	if configuration.Storage.S3Enabled || configuration.Storage.LocalDir != "tmp/files" {
		t.Fatalf("Storage overrides = %#v", configuration.Storage)
	}
	// Embedding 超时单独检查，避免外部模型请求使用错误默认值。
	if configuration.Embedding.Timeout != 20*time.Second {
		t.Fatalf("Embedding.Timeout = %s, want 20s", configuration.Embedding.Timeout)
	}
	// 2KB 要换算成字节，扩展名还要统一去点并转成小写。
	if configuration.Document.MaxUploadBytes != 2*1024 ||
		configuration.Document.IsExtensionAllowed("pdf") ||
		!configuration.Document.IsExtensionAllowed("md") {
		t.Fatalf("Document overrides = %#v", configuration.Document)
	}
	// 日志级别和格式最终决定生产输出内容，也必须允许环境变量覆盖。
	if configuration.Log.Level != "debug" || configuration.Log.Format != "text" {
		t.Fatalf("Log overrides = %#v", configuration.Log)
	}
}

// TestLoadRejectsInvalidDuration 验证拼错的超时配置会在启动阶段报出具体环境变量名。
func TestLoadRejectsInvalidDuration(t *testing.T) {
	// sixty 不是 time.ParseDuration 可识别的格式，Load 应在启动前直接返回错误。
	_, err := Load(mapGetenv(map[string]string{"HTTP_READ_TIMEOUT": "sixty"}))
	if err == nil {
		t.Fatal("Load() 应拒绝非法 duration")
	}
	// 错误带出变量名后，运维人员才能快速定位是哪一项配置写错。
	if !strings.Contains(err.Error(), "HTTP_READ_TIMEOUT") {
		t.Fatalf("error = %q, want mention HTTP_READ_TIMEOUT", err)
	}
}

// TestValidateReportsMissingRequiredConfiguration 验证外部依赖所需配置会被集中校验，而不是连接时零散失败。
func TestValidateReportsMissingRequiredConfiguration(t *testing.T) {
	// base 是能通过校验的完整起点，每个子场景只删除一个必填变量。
	base := validEnvironment()
	tests := []struct {
		// name 说明缺少的是哪项外部依赖配置。
		name string
		// removeKey 是要从合法环境中删除的变量名。
		removeKey string
		// wantInError 要求错误明确提到缺失变量，方便部署排查。
		wantInError string
	}{
		{name: "缺少数据库 DSN", removeKey: "POSTGRESQL_DSN", wantInError: "POSTGRESQL_DSN"},
		{name: "缺少 Embedding API key", removeKey: "EMBEDDING_API_KEY", wantInError: "EMBEDDING_API_KEY"},
		{name: "S3 缺少 bucket", removeKey: "RUSTFS_BUCKET_NAME", wantInError: "RUSTFS_BUCKET_NAME"},
		{name: "S3 缺少 access key", removeKey: "RUSTFS_ACCESS_KEY", wantInError: "RUSTFS_ACCESS_KEY"},
		{name: "S3 缺少 secret key", removeKey: "RUSTFS_SECRET_KEY", wantInError: "RUSTFS_SECRET_KEY"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// 每个子测试复制一份 map，避免删除键后污染其他场景。
			values := cloneStrings(base)
			delete(values, test.removeKey)
			// Load 负责解析，缺少必填项由后续集中 Validate 统一报告。
			configuration, err := Load(mapGetenv(values))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			// 校验必须失败且包含变量名，不能只返回笼统的“配置错误”。
			if err := configuration.Validate(); err == nil || !strings.Contains(err.Error(), test.wantInError) {
				t.Fatalf("Validate() error = %v, want mention %s", err, test.wantInError)
			}
		})
	}
}

// TestValidateAllowsLocalStorageWithoutS3Credentials 验证明确关闭 S3 后，本地开发不必伪造 bucket 或密钥。
func TestValidateAllowsLocalStorageWithoutS3Credentials(t *testing.T) {
	// 从合法 S3 环境切换为本地存储，并主动移除所有 S3 专用字段。
	values := validEnvironment()
	values["RUSTFS_ENABLED"] = "false"
	values["LOCAL_STORAGE_DIR"] = ".data/test-storage"
	delete(values, "RUSTFS_BUCKET_NAME")
	delete(values, "RUSTFS_ACCESS_KEY")
	delete(values, "RUSTFS_SECRET_KEY")

	// 本地模式仍需要数据库和 Embedding，但不应强迫开发者伪造对象存储密钥。
	configuration, err := Load(mapGetenv(values))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := configuration.Validate(); err != nil {
		t.Fatalf("本地存储配置 Validate() error = %v", err)
	}
}

// mapGetenv 把普通 map 包装成与 os.Getenv 相同形状的函数，供测试精确控制输入。
func mapGetenv(values map[string]string) func(string) string {
	// map 读取不存在的键会返回空字符串，行为与 os.Getenv 对未设置变量的结果一致。
	return func(key string) string {
		return values[key]
	}
}

// validEnvironment 返回能够通过集中校验的最小外部依赖配置。
func validEnvironment() map[string]string {
	// 只放集中校验要求的字段，其余可选项继续使用默认值。
	return map[string]string{
		"POSTGRESQL_DSN":     "postgres://example",
		"EMBEDDING_API_KEY":  "test-key",
		"RUSTFS_ENABLED":     "true",
		"RUSTFS_BUCKET_NAME": "knowledge",
		"RUSTFS_ACCESS_KEY":  "access",
		"RUSTFS_SECRET_KEY":  "secret",
	}
}

// cloneStrings 为表驱动测试复制环境变量 map，避免测试之间共享可变状态。
func cloneStrings(source map[string]string) map[string]string {
	// 新 map 拥有独立键值集合，后续 delete 不会改动源环境。
	cloned := make(map[string]string, len(source))
	// 字符串是值类型，逐项复制即可完成安全隔离。
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
