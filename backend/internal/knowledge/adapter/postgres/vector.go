package postgres

import (
	"fmt"
	"strconv"
	"strings"
)

// 本文件只负责 pgvector 文本格式转换，不负责决定何时调用 Embedding 或何时开启事务。
// vectorText 把 Go 的浮点切片转换成 pgvector 接受的文本字面量。
//
// expectedDimensions 由调用方传入，生产仓储固定传 1536；测试可以传较小维度检查格式。
// 维度校验必须发生在执行 SQL 之前，否则 PostgreSQL 只能返回难懂的类型错误。
func vectorText(vector []float32, expectedDimensions int) (string, error) {
	// 空向量没有任何检索意义，也无法满足数据库 vector(N) 类型要求。
	if len(vector) == 0 {
		return "", fmt.Errorf("embedding vector is empty")
	}
	// 期望维度本身必须是正数，避免调用方配置错误被误报成向量内容问题。
	if expectedDimensions <= 0 {
		return "", fmt.Errorf("expected embedding dimensions must be positive")
	}
	// 向量长度必须与数据库列维度完全一致，否则 PostgreSQL 会拒绝写入。
	if len(vector) != expectedDimensions {
		return "", fmt.Errorf(
			"embedding dimensions %d do not match required %d",
			len(vector),
			expectedDimensions,
		)
	}

	// 预先分配固定长度的字符串切片，循环里只填值，避免不断 append 扩容。
	parts := make([]string, len(vector))
	// 保持原切片顺序逐项格式化，任何顺序变化都会改变向量含义。
	for index, value := range vector {
		// bitSize=32 表示按 float32 的真实精度输出，避免先转 float64 后出现无意义的小数尾巴。
		parts[index] = strconv.FormatFloat(float64(value), 'f', -1, 32)
	}

	// pgvector 文本格式示例是 [0.1,-2,3.25]，元素之间不能使用 Go 默认格式产生的空格。
	return "[" + strings.Join(parts, ",") + "]", nil
}
