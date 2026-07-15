package domain

import "errors"

// 这些错误是跨应用层和 HTTP 适配器都需要稳定识别的业务类别。
// 外层可以使用 errors.Is 判断类别，再决定 HTTP 状态码；底层错误的技术细节仍通过 %w 保留。
var (
	// ErrNotFound 表示请求的知识库或文档不存在。
	ErrNotFound = errors.New("not found")
	// ErrDuplicate 表示同一知识库已经存在内容哈希相同的有效文档。
	ErrDuplicate = errors.New("同一知识库已存在相同内容的文档")
	// ErrAlreadyRunning 表示另一个请求已经取得文档处理权，本次不能重复处理。
	ErrAlreadyRunning = errors.New("文档正在处理中")
	// ErrInvalidArgument 表示调用方传入的数据不满足业务约束。
	ErrInvalidArgument = errors.New("invalid argument")
	// ErrInvalidTransition 表示文档当前状态不允许执行目标状态转换。
	ErrInvalidTransition = errors.New("invalid document status transition")
)
