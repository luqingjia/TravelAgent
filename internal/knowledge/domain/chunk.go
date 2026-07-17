package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	// defaultMinChunkChars 表示普通分块希望达到的最小字符数，过短片段会优先与相邻内容合并。
	defaultMinChunkChars = 100
	// defaultTargetChunkChars 是算法尽量靠近的目标大小，在语义完整与向量检索粒度之间取平衡。
	defaultTargetChunkChars = 800
	// defaultMaxChunkChars 是硬上限，超长段落会继续切分，防止单个向量承载过多文本。
	defaultMaxChunkChars = 1200
)

// Chunk 表示从一个知识文档中按顺序切出的文本片段。
//
// StartPosition 和 EndPosition 使用原始 UTF-8 字节下标，因为 Go 的字符串切片按字节工作；
// CharCount 则使用 Unicode 字符数，供接口展示和近似 token 统计使用，二者不能混为一谈。
type Chunk struct {
	// ID 是分块自己的唯一标识，由应用层在持久化前生成。
	ID string
	// KbID 表示分块属于哪个知识库，便于数据库按知识库过滤。
	KbID string
	// DocumentID 表示分块来自哪个原始文档。
	DocumentID string
	// Index 是分块在原文中的顺序，从 0 开始连续递增。
	Index int
	// Content 保存真正送入 Embedding 和检索的数据。
	Content string
	// TokenCount 当前使用 Unicode 字符数近似，未来接入 tokenizer 后可以替换计算方式。
	TokenCount int
	// CharCount 是 Content 中 Unicode 字符的数量，不是 UTF-8 字节数。
	CharCount int
	// StartPosition 是 Content 在原始字符串中的起始字节下标。
	StartPosition int
	// EndPosition 是 Content 在原始字符串中的结束字节下标，采用左闭右开区间。
	EndPosition int
	// Metadata 保存分块扩展信息；领域层不关心它最终怎样序列化成 JSONB。
	Metadata map[string]any
}

// Validate 检查一个分块是否已经具备进入持久化层所需的完整信息。
// 分块算法刚生成片段时还没有 ID 和归属信息；应用服务补齐这些字段后，必须在写数据库前调用本方法。
func (c Chunk) Validate() error {
	// 没有 ID 的分块无法被向量表稳定引用，因此不能进入仓储层。
	if strings.TrimSpace(c.ID) == "" {
		return fmt.Errorf("%w: chunk id is empty", ErrInvalidArgument)
	}
	// 知识库 ID 缺失会破坏知识库范围查询。
	if strings.TrimSpace(c.KbID) == "" {
		return fmt.Errorf("%w: chunk knowledge base id is empty", ErrInvalidArgument)
	}
	// 文档 ID 缺失后无法知道分块来源，也无法执行整篇文档替换。
	if strings.TrimSpace(c.DocumentID) == "" {
		return fmt.Errorf("%w: chunk document id is empty", ErrInvalidArgument)
	}
	// 顺序下标必须从 0 开始，负数没有业务意义。
	if c.Index < 0 {
		return fmt.Errorf("%w: chunk index must not be negative", ErrInvalidArgument)
	}
	// 空白分块既浪费 Embedding 调用，也无法为检索提供内容。
	if strings.TrimSpace(c.Content) == "" {
		return fmt.Errorf("%w: chunk content is empty", ErrInvalidArgument)
	}
	// token 近似计数不能为负数，否则统计信息会失真。
	if c.TokenCount < 0 {
		return fmt.Errorf("%w: token count must not be negative", ErrInvalidArgument)
	}
	// 重新计算真实字符数，防止调用方传入与内容不一致的派生字段。
	if c.CharCount != utf8.RuneCountInString(c.Content) {
		return fmt.Errorf("%w: char count %d does not match content", ErrInvalidArgument, c.CharCount)
	}
	// 原文区间必须是有效的左闭右开范围，并且至少包含一个字节。
	if c.StartPosition < 0 || c.EndPosition <= c.StartPosition {
		return fmt.Errorf("%w: invalid chunk positions %d..%d", ErrInvalidArgument, c.StartPosition, c.EndPosition)
	}
	// 原文位置按字节保存，因此区间长度必须等于 Content 的字节长度，才能保证之后从原文切片得到同一内容。
	if c.EndPosition-c.StartPosition != len(c.Content) {
		return fmt.Errorf("%w: chunk position length does not match content", ErrInvalidArgument)
	}
	// 所有不变量都通过后，当前分块才可以安全交给数据库适配器。
	return nil
}

// ChunkOptions 是结构感知分块使用的三个字符阈值。
// MinChars <= TargetChars <= MaxChars 是算法能够稳定工作的必要范围关系。
type ChunkOptions struct {
	// MinChars 控制普通分块允许的最小目标大小。
	MinChars int
	// TargetChars 是算法优先尝试达到的大小。
	TargetChars int
	// MaxChars 是任何分块都不能超过的硬上限。
	MaxChars int
}

// Normalize 为未填写的零值补默认值，并拒绝负数或互相矛盾的范围。
// 统一在这里处理后，HTTP、应用服务和分块算法就不需要各写一份默认值逻辑。
func (o ChunkOptions) Normalize() (ChunkOptions, error) {
	// 负数不是“未填写”，而是明确的非法输入，所以先于默认值处理进行拒绝。
	if o.MinChars < 0 || o.TargetChars < 0 || o.MaxChars < 0 {
		return ChunkOptions{}, fmt.Errorf("%w: chunk thresholds must not be negative", ErrInvalidArgument)
	}
	// 零值表示调用方没有指定最小值，此时采用系统默认配置。
	if o.MinChars == 0 {
		o.MinChars = defaultMinChunkChars
	}
	// 零值目标大小使用经过当前业务验证的 800 字符默认值。
	if o.TargetChars == 0 {
		o.TargetChars = defaultTargetChunkChars
	}
	// 零值最大大小使用 1200 字符上限，避免生成无限增长的分块。
	if o.MaxChars == 0 {
		o.MaxChars = defaultMaxChunkChars
	}
	// 最小值大于目标值时，算法无法同时满足两个要求。
	if o.MinChars > o.TargetChars {
		return ChunkOptions{}, fmt.Errorf("%w: minChars must not exceed targetChars", ErrInvalidArgument)
	}
	// 目标值大于最大值时，算法刚达到目标就已经越过硬上限。
	if o.TargetChars > o.MaxChars {
		return ChunkOptions{}, fmt.Errorf("%w: targetChars must not exceed maxChars", ErrInvalidArgument)
	}
	// 返回归一化后的新值，原调用方持有的结构体不会被偷偷修改。
	return o, nil
}

// AsMap 把已经归一化的选项转换成数据库 chunk_config 使用的稳定字段名。
// 返回新 map，调用方可以安全地继续补充字段，不会反向修改 ChunkOptions 值。
func (o ChunkOptions) AsMap() map[string]any {
	// 字段名保持现有 HTTP 和数据库兼容格式，不能随 Go 字段命名随意变化。
	return map[string]any{
		"minChars":    o.MinChars,
		"targetChars": o.TargetChars,
		"maxChars":    o.MaxChars,
	}
}
