package httpadapter

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// 本文件把 Gin 收到的请求格式转换成领域层能理解的普通 Go 值，
// JSON tag、表单字段名和 gin.Context 都不会越过 HTTP 适配器边界。
// chunkRequest 是分块接口接收的 HTTP JSON 模型。
// JSON tag 只存在于适配层；转换后应用层拿到不依赖 Gin 的 domain.ChunkOptions。
type chunkRequest struct {
	// MinChars 是客户端可选的最小分块字符数，零值交给领域层补默认值。
	MinChars int `json:"minChars"`
	// TargetChars 是客户端希望接近的分块大小。
	TargetChars int `json:"targetChars"`
	// MaxChars 是客户端允许的单块最大字符数。
	MaxChars int `json:"maxChars"`
}

// toDomain 把请求 DTO 转换成领域选项；默认值和范围关系仍由 domain.Normalize 统一处理。
func (request chunkRequest) toDomain() domain.ChunkOptions {
	// 这里只做字段一一映射，不在 HTTP 层复制默认值和范围校验规则。
	return domain.ChunkOptions{
		MinChars:    request.MinChars,
		TargetChars: request.TargetChars,
		MaxChars:    request.MaxChars,
	}
}

// optionalJSONFormMap 解析 multipart 中可选的 JSON 对象字段。
// 旧调用方不提供时返回空 map；提供但格式错误时映射成稳定参数错误，不让 JSON 细节进入应用层。
func optionalJSONFormMap(context *gin.Context, field string) (map[string]any, error) {
	// PostForm 会读取 multipart 普通字段；TrimSpace 让纯空白和未提供具有相同语义。
	raw := strings.TrimSpace(context.PostForm(field))
	// 可选字段缺失时返回可直接使用的空 map，调用方不需要额外判断 nil。
	if raw == "" {
		return map[string]any{}, nil
	}
	// 预先创建目标 map，保证合法空对象 {} 也能得到非 nil 结果。
	decoded := map[string]any{}
	// JSON 必须是对象形状；解析失败统一包装成可由 HTTP 层识别的参数错误。
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil, fmt.Errorf("%w: multipart field %s must be a JSON object", domain.ErrInvalidArgument, field)
	}
	// JSON 文本 null 会把 map 解码成 nil，这里把它归一化为空对象，维持返回合同。
	if decoded == nil {
		return map[string]any{}, nil
	}
	// 返回独立解码结果，后续领域对象复制时不会再依赖 gin.Context。
	return decoded, nil
}
