package application

import (
	"strings"
	"testing"

	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// TestChunkTextKeepsOrderAndPositions 迁移旧 MVP 的核心回归场景：顺序、位置和原文切片必须完全对应。
func TestChunkTextKeepsOrderAndPositions(t *testing.T) {
	// 准备：三段中文文本用空行分隔，阈值刻意设置得较小，确保能产生多个分块。
	text := "第一段内容，适合放在第一个块。\n\n第二段内容会继续被切分。\n\n第三段内容用于确认顺序。"

	// 执行：分块算法只处理纯文本和领域选项，不访问数据库或外部模型。
	chunks, err := ChunkText(text, domain.ChunkOptions{MinChars: 8, TargetChars: 24, MaxChars: 48})
	if err != nil {
		t.Fatalf("ChunkText() error = %v", err)
	}

	// 断言：至少切成两块，索引连续，且每块内容都能用保存的字节位置从原文精确还原。
	if len(chunks) < 2 {
		t.Fatalf("chunk count = %d, want at least 2", len(chunks))
	}
	for index, chunk := range chunks {
		if chunk.Index != index {
			t.Fatalf("chunk index = %d, want %d", chunk.Index, index)
		}
		if chunk.StartPosition < 0 || chunk.EndPosition <= chunk.StartPosition {
			t.Fatalf("invalid positions for chunk %d: %#v", index, chunk)
		}
		if got := text[chunk.StartPosition:chunk.EndPosition]; got != chunk.Content {
			t.Fatalf("chunk %d content = %q, original slice = %q", index, chunk.Content, got)
		}
	}
}

// TestChunkTextReturnsNoChunksForBlankInput 验证纯空白解析结果不会进入 Embedding。
func TestChunkTextReturnsNoChunksForBlankInput(t *testing.T) {
	chunks, err := ChunkText(" \n\t ", domain.ChunkOptions{})
	if err != nil {
		t.Fatalf("ChunkText() error = %v", err)
	}
	if len(chunks) != 0 {
		t.Fatalf("blank text chunks = %d, want 0", len(chunks))
	}
}

// TestChunkTextRejectsInvalidOptions 验证范围错误在算法开始前直接返回，不产生看似成功但不可解释的结果。
func TestChunkTextRejectsInvalidOptions(t *testing.T) {
	_, err := ChunkText("hello", domain.ChunkOptions{MinChars: 20, TargetChars: 10, MaxChars: 30})
	if err == nil {
		t.Fatal("ChunkText() error = nil, want invalid options error")
	}
}

// TestChunkTextSplitsOversizedParagraph 验证没有空行的超长段落也不会突破最大块限制。
func TestChunkTextSplitsOversizedParagraph(t *testing.T) {
	// 准备：使用 ASCII 让字节位置和字符数量都一目了然，25 个字符在 max=10 时应被拆成三块。
	text := "abcdefghijklmnopqrstuvwxy"

	chunks, err := ChunkText(text, domain.ChunkOptions{MinChars: 1, TargetChars: 10, MaxChars: 10})
	if err != nil {
		t.Fatalf("ChunkText() error = %v", err)
	}

	if len(chunks) != 3 {
		t.Fatalf("chunk count = %d, want 3", len(chunks))
	}
	var rebuilt strings.Builder
	for _, chunk := range chunks {
		if len(chunk.Content) > 10 {
			t.Fatalf("chunk %d length = %d, want <= 10", chunk.Index, len(chunk.Content))
		}
		rebuilt.WriteString(chunk.Content)
	}
	if rebuilt.String() != text {
		t.Fatalf("rebuilt text = %q, want %q", rebuilt.String(), text)
	}
}
