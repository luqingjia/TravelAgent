package postgres

import (
	"reflect"
	"testing"
	"time"

	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// TestDocumentRowRoundTrip 验证领域对象先转成数据库行，再从数据库行恢复后不会丢字段。
// 这个场景专门防止拆分 domain 与 SQL 模型时漏掉状态、JSON 配置或时间等不容易察觉的数据。
func TestDocumentRowRoundTrip(t *testing.T) {
	// 准备：构造一份字段尽量完整的已完成文档。JSON 数字使用 float64，和 encoding/json
	// 解码到 map[string]any 后的实际类型一致，测试关注的是数据是否保存，而不是 Go 数字类型转换细节。
	createdAt := time.Date(2026, time.July, 14, 9, 30, 0, 0, time.UTC)
	updatedAt := createdAt.Add(5 * time.Minute)
	original, err := domain.RestoreDocument(domain.Document{
		ID:            "doc-1",
		KbID:          "kb-1",
		Title:         "企业级 Go 项目说明",
		SourceType:    domain.SourceTypeFile,
		SourceURI:     "s3://knowledge/doc-1.md",
		FileName:      "travel-agent.md",
		FileType:      "md",
		FileSize:      2048,
		ContentHash:   "sha256-value",
		Language:      "zh",
		Status:        domain.StatusCompleted,
		ChunkCount:    2,
		ChunkStrategy: domain.DefaultChunkStrategy,
		ChunkConfig: map[string]any{
			"targetChars": float64(800),
			"nested":      map[string]any{"keep": true},
		},
		Metadata: map[string]any{
			"contentType": "text/markdown",
			"labels":      []any{"go", "ddd"},
		},
		CreateTime: createdAt,
		UpdateTime: updatedAt,
	})
	if err != nil {
		t.Fatalf("准备领域文档失败：%v", err)
	}

	// 执行：rowFromDomain 模拟写库前的显式转换，toDomain 模拟 sqlx 扫描后的恢复过程。
	row, err := rowFromDomain(original)
	if err != nil {
		t.Fatalf("rowFromDomain() error = %v", err)
	}
	restored, err := row.toDomain()
	if err != nil {
		t.Fatalf("documentRow.toDomain() error = %v", err)
	}

	// 关键断言：逐个标量字段比较，任何数据库列映射遗漏都会明确指出是哪一个字段发生了漂移。
	if restored.ID != original.ID ||
		restored.KbID != original.KbID ||
		restored.Title != original.Title ||
		restored.SourceType != original.SourceType ||
		restored.SourceURI != original.SourceURI ||
		restored.FileName != original.FileName ||
		restored.FileType != original.FileType ||
		restored.FileSize != original.FileSize ||
		restored.ContentHash != original.ContentHash ||
		restored.Language != original.Language ||
		restored.Status != original.Status ||
		restored.ChunkCount != original.ChunkCount ||
		restored.ChunkStrategy != original.ChunkStrategy ||
		!restored.CreateTime.Equal(original.CreateTime) ||
		!restored.UpdateTime.Equal(original.UpdateTime) {
		t.Fatalf("恢复后的标量字段不一致：\n got  = %#v\n want = %#v", restored, original)
	}

	// JSON 列必须完整往返；这里还覆盖嵌套 map 和 slice，避免只验证最简单的扁平对象。
	if !reflect.DeepEqual(restored.ChunkConfig, original.ChunkConfig) {
		t.Fatalf("ChunkConfig = %#v, want %#v", restored.ChunkConfig, original.ChunkConfig)
	}
	if !reflect.DeepEqual(restored.Metadata, original.Metadata) {
		t.Fatalf("Metadata = %#v, want %#v", restored.Metadata, original.Metadata)
	}
}

// TestDocumentRowRejectsInvalidJSON 验证数据库 JSON 列损坏时不能恢复出一个半可信的领域对象。
func TestDocumentRowRejectsInvalidJSON(t *testing.T) {
	// 准备：只需要让 chunk_config 成为非法 JSON；其余字段即使完整，也不应掩盖数据损坏。
	row := documentRow{ChunkConfig: []byte("{")}

	// 执行并断言：转换必须返回错误，调用方随后可以停止业务处理并暴露真实数据问题。
	if _, err := row.toDomain(); err == nil {
		t.Fatal("documentRow.toDomain() 应拒绝非法 chunk_config JSON")
	}
}
