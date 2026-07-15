package postgres

import (
	"fmt"
	"strconv"
	"strings"
)

// vectorText 把 Go 的浮点切片转换成 pgvector 接受的文本字面量。
//
// expectedDimensions 由调用方传入，生产仓储固定传 1536；测试可以传较小维度检查格式。
// 维度校验必须发生在执行 SQL 之前，否则 PostgreSQL 只能返回难懂的类型错误。
func vectorText(vector []float32, expectedDimensions int) (string, error) {
	if len(vector) == 0 {
		return "", fmt.Errorf("embedding vector is empty")
	}
	if expectedDimensions <= 0 {
		return "", fmt.Errorf("expected embedding dimensions must be positive")
	}
	if len(vector) != expectedDimensions {
		return "", fmt.Errorf(
			"embedding dimensions %d do not match required %d",
			len(vector),
			expectedDimensions,
		)
	}

	// 预先分配固定长度的字符串切片，循环里只填值，避免不断 append 扩容。
	parts := make([]string, len(vector))
	for index, value := range vector {
		// bitSize=32 表示按 float32 的真实精度输出，避免先转 float64 后出现无意义的小数尾巴。
		parts[index] = strconv.FormatFloat(float64(value), 'f', -1, 32)
	}

	// pgvector 文本格式示例是 [0.1,-2,3.25]，元素之间不能使用 Go 默认格式产生的空格。
	return "[" + strings.Join(parts, ",") + "]", nil
}
