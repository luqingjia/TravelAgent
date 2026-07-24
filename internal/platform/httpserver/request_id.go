// Package httpserver 提供与具体业务无关的 Gin 请求级中间件和 HTTP Server 辅助能力。
package httpserver

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// RequestIDHeader 是客户端传入和服务端回传请求 ID 的标准头名。
	RequestIDHeader = "X-Request-ID"
	// requestIDContextKey 是 Gin Context 内部使用的私有键，避免与其他中间件字段冲突。
	requestIDContextKey = "travel_agent_request_id"
	// maximumRequestIDLength 防止攻击者把超长头写入每一条日志。
	maximumRequestIDLength = 128
)

// RequestID 从 Gin 请求上下文读取经过校验的请求 ID；没有中间件时返回空字符串。
func RequestID(context *gin.Context) string {
	// 中间件会把校验后的值放在私有键下；这里不直接重新读取不可信请求头。
	value, exists := context.Get(requestIDContextKey)
	// 单元测试或特殊调用没有经过 RequestID 中间件时，空字符串表示当前没有请求 ID。
	if !exists {
		return ""
	}
	// 类型断言失败时得到空字符串，避免因为其他中间件误写同名键而 panic。
	requestID, _ := value.(string)
	// 调用方只需要稳定字符串，不需要知道 Gin Context 的存储细节。
	return requestID
}

// validRequestID 只允许适合放入日志和响应头的短 ASCII 字符，阻止换行等日志注入字符。
func validRequestID(value string) bool {
	// 空值由服务端重新生成；超长值会放大日志和响应头，所以同样拒绝。
	if value == "" || len(value) > maximumRequestIDLength {
		return false
	}
	// 逐个 Unicode 字符检查，实际允许范围严格限制为常用 ASCII 标识字符。
	for _, character := range value {
		// 字母、数字以及 -_.: 足以覆盖 UUID、trace ID 和常见网关请求 ID。
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune("-_.:", character) {
			// 当前字符合法时继续检查下一个字符。
			continue
		}
		// 发现空格、控制符、换行或其他符号时立即判定整个值不安全。
		return false
	}
	// 所有字符都通过白名单后，这个值才可以写入日志和响应头。
	return true
}

// generateRequestID 生成 128 位随机十六进制 ID；极少数随机源失败时使用纳秒时间兜底。
func generateRequestID() string {
	// 16 字节随机数等于 128 位随机空间，编码后得到 32 位十六进制字符串。
	var random [16]byte
	// 优先使用系统密码学随机源，正常生产环境都会走这个分支。
	if _, err := rand.Read(random[:]); err == nil {
		// 十六进制只包含安全 ASCII 字符，可以直接进入响应头和结构化日志。
		return hex.EncodeToString(random[:])
	}
	// 随机源异常时仍需给请求一个可关联标识；时间兜底不承担安全令牌用途。
	return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
}
