package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	defaultMinChunkChars    = 100
	defaultTargetChunkChars = 800
	defaultMaxChunkChars    = 1200
)

// Chunk 表示从一个知识文档中按顺序切出的文本片段。
//
// StartPosition 和 EndPosition 使用原始 UTF-8 字节下标，因为 Go 的字符串切片按字节工作；
// CharCount 则使用 Unicode 字符数，供接口展示和近似 token 统计使用，二者不能混为一谈。
type Chunk struct {
	ID            string
	KbID          string
	DocumentID    string
	Index         int
	Content       string
	TokenCount    int
	CharCount     int
	StartPosition int
	EndPosition   int
	Metadata      map[string]any
}

// Validate 检查一个分块是否已经具备进入持久化层所需的完整信息。
// 分块算法刚生成片段时还没有 ID 和归属信息；应用服务补齐这些字段后，必须在写数据库前调用本方法。
func (c Chunk) Validate() error {
	if strings.TrimSpace(c.ID) == "" {
		return fmt.Errorf("%w: chunk id is empty", ErrInvalidArgument)
	}
	if strings.TrimSpace(c.KbID) == "" {
		return fmt.Errorf("%w: chunk knowledge base id is empty", ErrInvalidArgument)
	}
	if strings.TrimSpace(c.DocumentID) == "" {
		return fmt.Errorf("%w: chunk document id is empty", ErrInvalidArgument)
	}
	if c.Index < 0 {
		return fmt.Errorf("%w: chunk index must not be negative", ErrInvalidArgument)
	}
	if strings.TrimSpace(c.Content) == "" {
		return fmt.Errorf("%w: chunk content is empty", ErrInvalidArgument)
	}
	if c.TokenCount < 0 {
		return fmt.Errorf("%w: token count must not be negative", ErrInvalidArgument)
	}
	if c.CharCount != utf8.RuneCountInString(c.Content) {
		return fmt.Errorf("%w: char count %d does not match content", ErrInvalidArgument, c.CharCount)
	}
	if c.StartPosition < 0 || c.EndPosition <= c.StartPosition {
		return fmt.Errorf("%w: invalid chunk positions %d..%d", ErrInvalidArgument, c.StartPosition, c.EndPosition)
	}
	// 原文位置按字节保存，因此区间长度必须等于 Content 的字节长度，才能保证之后从原文切片得到同一内容。
	if c.EndPosition-c.StartPosition != len(c.Content) {
		return fmt.Errorf("%w: chunk position length does not match content", ErrInvalidArgument)
	}
	return nil
}

// ChunkOptions 是结构感知分块使用的三个字符阈值。
// MinChars <= TargetChars <= MaxChars 是算法能够稳定工作的必要范围关系。
type ChunkOptions struct {
	MinChars    int
	TargetChars int
	MaxChars    int
}

// Normalize 为未填写的零值补默认值，并拒绝负数或互相矛盾的范围。
// 统一在这里处理后，HTTP、应用服务和分块算法就不需要各写一份默认值逻辑。
func (o ChunkOptions) Normalize() (ChunkOptions, error) {
	if o.MinChars < 0 || o.TargetChars < 0 || o.MaxChars < 0 {
		return ChunkOptions{}, fmt.Errorf("%w: chunk thresholds must not be negative", ErrInvalidArgument)
	}
	if o.MinChars == 0 {
		o.MinChars = defaultMinChunkChars
	}
	if o.TargetChars == 0 {
		o.TargetChars = defaultTargetChunkChars
	}
	if o.MaxChars == 0 {
		o.MaxChars = defaultMaxChunkChars
	}
	if o.MinChars > o.TargetChars {
		return ChunkOptions{}, fmt.Errorf("%w: minChars must not exceed targetChars", ErrInvalidArgument)
	}
	if o.TargetChars > o.MaxChars {
		return ChunkOptions{}, fmt.Errorf("%w: targetChars must not exceed maxChars", ErrInvalidArgument)
	}
	return o, nil
}

// AsMap 把已经归一化的选项转换成数据库 chunk_config 使用的稳定字段名。
// 返回新 map，调用方可以安全地继续补充字段，不会反向修改 ChunkOptions 值。
func (o ChunkOptions) AsMap() map[string]any {
	return map[string]any{
		"minChars":    o.MinChars,
		"targetChars": o.TargetChars,
		"maxChars":    o.MaxChars,
	}
}
