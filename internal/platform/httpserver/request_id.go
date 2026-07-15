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
	value, exists := context.Get(requestIDContextKey)
	if !exists {
		return ""
	}
	requestID, _ := value.(string)
	return requestID
}

// validRequestID 只允许适合放入日志和响应头的短 ASCII 字符，阻止换行等日志注入字符。
func validRequestID(value string) bool {
	if value == "" || len(value) > maximumRequestIDLength {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			strings.ContainsRune("-_.:", character) {
			continue
		}
		return false
	}
	return true
}

// generateRequestID 生成 128 位随机十六进制 ID；极少数随机源失败时使用纳秒时间兜底。
func generateRequestID() string {
	var random [16]byte
	if _, err := rand.Read(random[:]); err == nil {
		return hex.EncodeToString(random[:])
	}
	return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
}
