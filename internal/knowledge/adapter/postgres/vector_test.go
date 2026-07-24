package postgres

import (
	"strings"
	"testing"

	"github.com/luqingjia/TravelAgent/internal/knowledge/application"
)

// TestVectorTextFormatsPgvectorLiteral 验证普通浮点值会被转成 PostgreSQL pgvector 接受的文本字面量。
func TestVectorTextFormatsPgvectorLiteral(t *testing.T) {
	// 准备与执行：期望维度设为 3，便于直接检查格式，而不用在测试里硬编码 1536 个数字。
	got, err := vectorText([]float32{0.1, -2, 3.25}, 3)
	if err != nil {
		t.Fatalf("vectorText() error = %v", err)
	}

	// 关键断言：pgvector 要求方括号包裹、逗号分隔，不能输出 Go 默认的空格格式。
	if got != "[0.1,-2,3.25]" {
		t.Fatalf("vectorText() = %q, want %q", got, "[0.1,-2,3.25]")
	}
}

// TestVectorTextRejectsEmptyVector 验证空向量不会生成看似合法但无法入库的字符串。
func TestVectorTextRejectsEmptyVector(t *testing.T) {
	// nil 切片没有任何维度信息，写成 [] 会把错误推迟到数据库执行阶段。
	if _, err := vectorText(nil, application.EmbeddingDimensions); err == nil {
		t.Fatal("vectorText() 应拒绝空向量")
	}
}

// TestVectorTextValidatesEmbeddingDimensions 验证持久化边界继续强制现有数据库的 1536 维合同。
func TestVectorTextValidatesEmbeddingDimensions(t *testing.T) {
	// 准备：1535 维只差一位，最容易在更换模型配置时被忽略。
	wrong := make([]float32, application.EmbeddingDimensions-1)
	if _, err := vectorText(wrong, application.EmbeddingDimensions); err == nil {
		t.Fatalf("vectorText() 应拒绝 %d 维向量", len(wrong))
	}

	// 正确的 1536 维必须能生成完整字面量；这里只检查边界字符，避免脆弱地比较超长字符串。
	valid := make([]float32, application.EmbeddingDimensions)
	// 零值向量内容合法，本场景只验证维度和 pgvector 外层格式。
	got, err := vectorText(valid, application.EmbeddingDimensions)
	if err != nil {
		t.Fatalf("vectorText() valid vector error = %v", err)
	}
	if !strings.HasPrefix(got, "[") || !strings.HasSuffix(got, "]") {
		t.Fatalf("vectorText() 没有生成 pgvector 方括号格式：%q", got)
	}
}
