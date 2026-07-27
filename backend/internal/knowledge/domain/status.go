// Package domain 保存知识库业务最核心、最稳定的规则。
//
// 这个包只使用 Go 标准库，不认识 Gin、数据库、S3 或环境变量。这样做的直接好处是：
// 领域规则可以在纯内存中测试，也不会因为以后更换 HTTP 框架或数据库驱动而被迫改写。
package domain

// DocumentStatus 表示知识文档当前处于处理生命周期的哪一步。
// 使用自定义类型而不是散落的普通字符串，可以让函数签名直接表达业务含义，并集中校验合法值。
type DocumentStatus string

const (
	// StatusPending 表示文件已经上传并登记，但还没有开始解析和向量化。
	StatusPending DocumentStatus = "pending"
	// StatusProcessing 表示某个请求已经通过数据库原子更新取得了当前文档的处理权。
	StatusProcessing DocumentStatus = "processing"
	// StatusCompleted 表示分块和向量都已经在同一数据库事务中替换成功。
	StatusCompleted DocumentStatus = "completed"
	// StatusFailed 表示最近一次处理失败，具体原因保存在 metadata.lastError 中。
	StatusFailed DocumentStatus = "failed"
)

// Valid 判断状态值是不是当前业务明确支持的四种状态之一。
func (s DocumentStatus) Valid() bool {
	// 四个已声明常量都代表可以持久化和对外返回的合法生命周期状态。
	switch s {
	case StatusPending, StatusProcessing, StatusCompleted, StatusFailed:
		// 命中已知状态时直接返回 true，调用方可以继续恢复或转换领域对象。
		return true
	default:
		// 数据库脏值或调用方随意构造的字符串都会在这里被拒绝。
		return false
	}
}

// SourceType 表示知识内容最初从哪里进入系统。
// 目前 Go MVP 只支持上传文件，因此只定义真实存在的 file，不提前创建网页、数据库等占位类型。
type SourceType string

const (
	// SourceTypeFile 表示来源是用户上传的文件。
	SourceTypeFile SourceType = "file"
)

// Valid 判断来源类型是否是当前系统真正支持的值。
func (s SourceType) Valid() bool {
	// 当前 MVP 只有文件上传这一种真实来源，其他字符串不能伪装成受支持来源。
	return s == SourceTypeFile
}

const (
	// DefaultLanguage 是上传请求未指定语言时使用的默认值。
	DefaultLanguage = "zh"
	// DefaultChunkStrategy 保留现有 MVP 的默认分块策略名称，避免接口行为在重构时漂移。
	DefaultChunkStrategy = "structure_aware"
)
