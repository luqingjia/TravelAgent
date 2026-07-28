package einoadapter

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/luqingjia/TravelAgent/internal/agent/domain"
)

// 本文件注册 get_current_time 工具：只做本地时区转换，不访问网络。

// getCurrentTimeInput 是工具入参，timezone 必须是 IANA 时区名。
type getCurrentTimeInput struct {
	// Timezone 是 IANA 时区，例如 Asia/Shanghai 或 UTC。
	Timezone string `json:"timezone" jsonschema:"description=IANA timezone name such as Asia/Shanghai or UTC"`
}

// getCurrentTimeOutput 是工具出参，包含时区、RFC3339 和可读本地时间。
type getCurrentTimeOutput struct {
	// Timezone 是校验通过后的时区名。
	Timezone string `json:"timezone"`
	// RFC3339 是该时区下的 RFC3339 本地时间。
	RFC3339 string `json:"rfc3339"`
	// Readable 是便于人类阅读的本地时间文本。
	Readable string `json:"readable"`
}

// newGetCurrentTimeTool 创建 get_current_time 工具。
// 无效时区返回包装了 domain.ErrInvalidTimezone 的错误，便于上层分类。
func newGetCurrentTimeTool() (tool.BaseTool, error) {
	return utils.InferTool(
		"get_current_time",
		"Get the current local time for an IANA timezone",
		func(ctx context.Context, input getCurrentTimeInput) (getCurrentTimeOutput, error) {
			_ = ctx
			// 空时区无法定位，统一归类为非法参数。
			if input.Timezone == "" {
				return getCurrentTimeOutput{}, fmt.Errorf("%w: timezone is required", domain.ErrInvalidTimezone)
			}
			// LoadLocation 只查本地时区数据库，不发起网络请求。
			location, err := time.LoadLocation(input.Timezone)
			if err != nil {
				return getCurrentTimeOutput{}, fmt.Errorf("%w: %s", domain.ErrInvalidTimezone, input.Timezone)
			}
			now := time.Now().In(location)
			return getCurrentTimeOutput{
				Timezone: input.Timezone,
				RFC3339:  now.Format(time.RFC3339),
				Readable: now.Format("2006-01-02 15:04:05 MST"),
			}, nil
		},
	)
}
