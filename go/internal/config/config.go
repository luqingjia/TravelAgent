package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	defaultPort             = "8081"
	defaultEmbeddingBaseURL = "https://dashscope.aliyuncs.com/compatible-mode"
	defaultEmbeddingModel   = "text-embedding-v3"
	defaultEmbeddingDims    = 1536
	defaultAllowedExts      = "pdf,doc,docx,txt,md,markdown,html,htm"
	defaultMaxUploadSize    = "50MB"
)

type Config struct {
	Port      string
	Database  DatabaseConfig
	Storage   StorageConfig
	Embedding EmbeddingConfig
	Document  DocumentConfig
}

type DatabaseConfig struct {
	DSN string
}

type StorageConfig struct {
	S3Enabled bool
	Bucket    string
	Endpoint  string
	Region    string
	AccessKey string
	SecretKey string
	LocalDir  string
}

type EmbeddingConfig struct {
	APIKey     string
	BaseURL    string
	Model      string
	Dimensions int
}

type DocumentConfig struct {
	AllowedExtensions map[string]struct{}
	MaxUploadBytes    int64
}

func Load() (Config, error) {
	dimensions, err := intFromEnv("EMBEDDING_DIMENSIONS", defaultEmbeddingDims)
	if err != nil {
		return Config{}, err
	}
	maxUploadBytes, err := parseSizeBytes(valueOrDefault("KNOWLEDGE_DOCUMENT_MAX_SIZE", defaultMaxUploadSize))
	if err != nil {
		return Config{}, err
	}

	return Config{
		Port: valueOrDefault("GO_AGENT_PORT", defaultPort),
		Database: DatabaseConfig{
			DSN: os.Getenv("POSTGRESQL_DSN"),
		},
		Storage: StorageConfig{
			S3Enabled: boolFromEnv("RUSTFS_ENABLED", true),
			Bucket:    os.Getenv("RUSTFS_BUCKET_NAME"),
			Endpoint:  valueOrDefault("RUSTFS_ENDPOINT", "http://localhost:9000"),
			Region:    valueOrDefault("RUSTFS_REGION", "us-east-1"),
			AccessKey: os.Getenv("RUSTFS_ACCESS_KEY"),
			SecretKey: os.Getenv("RUSTFS_SECRET_KEY"),
			LocalDir:  valueOrDefault("LOCAL_STORAGE_DIR", ".data/storage"),
		},
		Embedding: EmbeddingConfig{
			APIKey:     os.Getenv("EMBEDDING_API_KEY"),
			BaseURL:    valueOrDefault("EMBEDDING_BASE_URL", defaultEmbeddingBaseURL),
			Model:      valueOrDefault("EMBEDDING_MODEL", defaultEmbeddingModel),
			Dimensions: dimensions,
		},
		Document: DocumentConfig{
			AllowedExtensions: parseExtensions(valueOrDefault("KNOWLEDGE_DOCUMENT_ALLOWED_EXTENSIONS", defaultAllowedExts)),
			MaxUploadBytes:    maxUploadBytes,
		},
	}, nil
}

func (c DocumentConfig) IsExtensionAllowed(ext string) bool {
	normalized := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(ext), "."))
	if normalized == "" {
		return false
	}
	_, ok := c.AllowedExtensions[normalized]
	return ok
}

func valueOrDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func intFromEnv(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return parsed, nil
}

func boolFromEnv(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	return value == "true" || value == "1" || value == "yes" || value == "y"
}

func parseExtensions(value string) map[string]struct{} {
	extensions := make(map[string]struct{})
	for _, item := range strings.Split(value, ",") {
		ext := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(item), "."))
		if ext != "" {
			extensions[ext] = struct{}{}
		}
	}
	return extensions
}

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
		{"GB", 1024 * 1024 * 1024},
		{"MB", 1024 * 1024},
		{"KB", 1024},
		{"B", 1},
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
	return amount * multiplier, nil
}
