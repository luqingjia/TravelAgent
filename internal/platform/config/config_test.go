package config

import (
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

	configuration, err := Load(mapGetenv(values))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if configuration.HTTP.Port != "9090" || configuration.HTTP.WriteTimeout != 2*time.Minute {
		t.Fatalf("HTTP overrides = %#v", configuration.HTTP)
	}
	if configuration.Database.MaxOpenConnections != 24 ||
		configuration.Database.MaxIdleConnections != 12 ||
		configuration.Database.ConnMaxLifetime != 45*time.Minute ||
		configuration.Database.ConnMaxIdleTime != 7*time.Minute {
		t.Fatalf("Database overrides = %#v", configuration.Database)
	}
	if configuration.Storage.S3Enabled || configuration.Storage.LocalDir != "tmp/files" {
		t.Fatalf("Storage overrides = %#v", configuration.Storage)
	}
	if configuration.Embedding.Timeout != 20*time.Second {
		t.Fatalf("Embedding.Timeout = %s, want 20s", configuration.Embedding.Timeout)
	}
	if configuration.Document.MaxUploadBytes != 2*1024 ||
		configuration.Document.IsExtensionAllowed("pdf") ||
		!configuration.Document.IsExtensionAllowed("md") {
		t.Fatalf("Document overrides = %#v", configuration.Document)
	}
	if configuration.Log.Level != "debug" || configuration.Log.Format != "text" {
		t.Fatalf("Log overrides = %#v", configuration.Log)
	}
}

// TestLoadRejectsInvalidDuration 验证拼错的超时配置会在启动阶段报出具体环境变量名。
func TestLoadRejectsInvalidDuration(t *testing.T) {
	_, err := Load(mapGetenv(map[string]string{"HTTP_READ_TIMEOUT": "sixty"}))
	if err == nil {
		t.Fatal("Load() 应拒绝非法 duration")
	}
	if !strings.Contains(err.Error(), "HTTP_READ_TIMEOUT") {
		t.Fatalf("error = %q, want mention HTTP_READ_TIMEOUT", err)
	}
}

// TestValidateReportsMissingRequiredConfiguration 验证外部依赖所需配置会被集中校验，而不是连接时零散失败。
func TestValidateReportsMissingRequiredConfiguration(t *testing.T) {
	base := validEnvironment()
	tests := []struct {
		name        string
		removeKey   string
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
			configuration, err := Load(mapGetenv(values))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if err := configuration.Validate(); err == nil || !strings.Contains(err.Error(), test.wantInError) {
				t.Fatalf("Validate() error = %v, want mention %s", err, test.wantInError)
			}
		})
	}
}

// TestValidateAllowsLocalStorageWithoutS3Credentials 验证明确关闭 S3 后，本地开发不必伪造 bucket 或密钥。
func TestValidateAllowsLocalStorageWithoutS3Credentials(t *testing.T) {
	values := validEnvironment()
	values["RUSTFS_ENABLED"] = "false"
	values["LOCAL_STORAGE_DIR"] = ".data/test-storage"
	delete(values, "RUSTFS_BUCKET_NAME")
	delete(values, "RUSTFS_ACCESS_KEY")
	delete(values, "RUSTFS_SECRET_KEY")

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
	return func(key string) string {
		return values[key]
	}
}

// validEnvironment 返回能够通过集中校验的最小外部依赖配置。
func validEnvironment() map[string]string {
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
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
