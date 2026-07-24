package domain_test

import (
	"testing"

	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// TestChunkOptionsNormalizeAppliesDefaults 验证空配置会得到与旧 Go MVP 一致的三个默认阈值。
func TestChunkOptionsNormalizeAppliesDefaults(t *testing.T) {
	// 执行：零值代表调用方没有提供配置，由领域规则补齐默认值。
	normalized, err := (domain.ChunkOptions{}).Normalize()
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	// 断言：默认值必须保持 100/800/1200，避免重构后同一份文本被切成不同结果。
	want := domain.ChunkOptions{MinChars: 100, TargetChars: 800, MaxChars: 1200}
	if normalized != want {
		t.Fatalf("normalized = %#v, want %#v", normalized, want)
	}
}

// TestChunkOptionsNormalizeRejectsInvalidRanges 验证明显错误的阈值在进入分块算法前就被拒绝。
func TestChunkOptionsNormalizeRejectsInvalidRanges(t *testing.T) {
	tests := []struct {
		name    string
		options domain.ChunkOptions
	}{
		{name: "负数", options: domain.ChunkOptions{MinChars: -1}},
		{name: "最小值大于目标值", options: domain.ChunkOptions{MinChars: 900, TargetChars: 800, MaxChars: 1200}},
		{name: "目标值大于最大值", options: domain.ChunkOptions{MinChars: 100, TargetChars: 1300, MaxChars: 1200}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 执行：所有范围校验集中在 Normalize，应用层不再重复写三套大小判断。
			_, err := tt.options.Normalize()

			// 断言：非法配置必须返回错误，不能悄悄产生难以解释的分块结果。
			if err == nil {
				t.Fatal("Normalize() error = nil, want validation error")
			}
		})
	}
}

// TestChunkOptionsAsMapUsesStableKeys 验证持久化到 chunk_config 的字段名保持现有 JSON 契约。
func TestChunkOptionsAsMapUsesStableKeys(t *testing.T) {
	// 使用一组完整阈值，避免默认值补齐干扰字段名合同测试。
	options := domain.ChunkOptions{MinChars: 100, TargetChars: 800, MaxChars: 1200}

	// AsMap 的结果最终会写入文档 chunk_config JSON。
	config := options.AsMap()

	// camelCase 键名和数值必须保持稳定，已有数据与客户端都依赖这些名称。
	if config["minChars"] != 100 || config["targetChars"] != 800 || config["maxChars"] != 1200 {
		t.Fatalf("config = %#v", config)
	}
}

// TestChunkValidateChecksPersistenceInvariants 验证进入数据库前的 Chunk 必须具有完整归属和合法原文位置。
func TestChunkValidateChecksPersistenceInvariants(t *testing.T) {
	// valid 代表准备进入数据库的最小完整分块，位置使用原文的字节边界。
	valid := domain.Chunk{
		ID:            "chunk-1",
		KbID:          "kb-1",
		DocumentID:    "doc-1",
		Index:         0,
		Content:       "第一段",
		TokenCount:    3,
		CharCount:     3,
		StartPosition: 0,
		EndPosition:   len("第一段"),
		Metadata:      map[string]any{},
	}

	// 先证明基准对象本身合法，后续子场景的失败才确实来自单项破坏。
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid chunk Validate() error = %v", err)
	}

	tests := []struct {
		// name 说明被破坏的领域不变量。
		name string
		// mutate 从合法副本出发只改一个字段。
		mutate func(domain.Chunk) domain.Chunk
	}{
		{name: "空编号", mutate: func(chunk domain.Chunk) domain.Chunk { chunk.ID = ""; return chunk }},
		{name: "负序号", mutate: func(chunk domain.Chunk) domain.Chunk { chunk.Index = -1; return chunk }},
		{name: "空内容", mutate: func(chunk domain.Chunk) domain.Chunk { chunk.Content = " "; return chunk }},
		{name: "结束位置不大于开始位置", mutate: func(chunk domain.Chunk) domain.Chunk { chunk.EndPosition = chunk.StartPosition; return chunk }},
		{name: "字符数量不一致", mutate: func(chunk domain.Chunk) domain.Chunk { chunk.CharCount = 99; return chunk }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 值类型复制保证每个子测试不会修改共享的 valid 基准。
			broken := tt.mutate(valid)
			// 非法分块必须在持久化前被拒绝，避免把坏位置或空内容写入数据库。
			if err := broken.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want validation error")
			}
		})
	}
}
