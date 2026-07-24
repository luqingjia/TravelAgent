package storage_test

import (
	"context"
	. "github.com/luqingjia/TravelAgent/internal/platform/storage"
	"strings"
	"testing"

	platformconfig "github.com/luqingjia/TravelAgent/internal/platform/config"
)

// TestNewS3StorageValidatesRequiredConfiguration 验证 S3/RustFS 适配器在构造阶段拒绝缺少关键配置。
func TestNewS3StorageValidatesRequiredConfiguration(t *testing.T) {
	base := platformconfig.Storage{
		Bucket:       "knowledge",
		Region:       "us-east-1",
		AccessKey:    "access",
		SecretKey:    "secret",
		Endpoint:     "http://127.0.0.1:9000",
		UsePathStyle: true,
	}

	tests := []struct {
		name        string
		mutate      func(*platformconfig.Storage)
		wantMessage string
	}{
		{name: "缺少 bucket", mutate: func(cfg *platformconfig.Storage) { cfg.Bucket = "" }, wantMessage: "RUSTFS_BUCKET_NAME"},
		{name: "缺少 region", mutate: func(cfg *platformconfig.Storage) { cfg.Region = "" }, wantMessage: "RUSTFS_REGION"},
		{name: "缺少 access key", mutate: func(cfg *platformconfig.Storage) { cfg.AccessKey = "" }, wantMessage: "RUSTFS_ACCESS_KEY"},
		{name: "缺少 secret key", mutate: func(cfg *platformconfig.Storage) { cfg.SecretKey = "" }, wantMessage: "RUSTFS_SECRET_KEY"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configuration := base
			tt.mutate(&configuration)
			_, err := NewS3Storage(context.Background(), configuration)
			if err == nil || !strings.Contains(err.Error(), tt.wantMessage) {
				t.Fatalf("NewS3Storage() error = %v, want mention %s", err, tt.wantMessage)
			}
		})
	}
}
