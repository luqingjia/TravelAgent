package httpadapter

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// chunkRequest 是分块接口接收的 HTTP JSON 模型。
// JSON tag 只存在于适配层；转换后应用层拿到不依赖 Gin 的 domain.ChunkOptions。
type chunkRequest struct {
	MinChars    int `json:"minChars"`
	TargetChars int `json:"targetChars"`
	MaxChars    int `json:"maxChars"`
}

// toDomain 把请求 DTO 转换成领域选项；默认值和范围关系仍由 domain.Normalize 统一处理。
func (request chunkRequest) toDomain() domain.ChunkOptions {
	return domain.ChunkOptions{
		MinChars:    request.MinChars,
		TargetChars: request.TargetChars,
		MaxChars:    request.MaxChars,
	}
}

// optionalJSONFormMap 解析 multipart 中可选的 JSON 对象字段。
// 旧调用方不提供时返回空 map；提供但格式错误时映射成稳定参数错误，不让 JSON 细节进入应用层。
func optionalJSONFormMap(context *gin.Context, field string) (map[string]any, error) {
	raw := strings.TrimSpace(context.PostForm(field))
	if raw == "" {
		return map[string]any{}, nil
	}
	decoded := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil, fmt.Errorf("%w: multipart field %s must be a JSON object", domain.ErrInvalidArgument, field)
	}
	if decoded == nil {
		return map[string]any{}, nil
	}
	return decoded, nil
}
