package application

import (
	"strings"
	"unicode/utf8"

	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// 本文件实现纯内存、可重复的结构感知分块算法，不访问数据库、存储或外部模型。
// textBlock 记录一个自然段在原始字符串中的半开区间 [start, end)。
// 使用字节下标是因为 Go 字符串切片按字节工作，最终可以无损地从原文恢复 Content。
type textBlock struct {
	// start 是自然段第一个非空白字符的 UTF-8 字节下标。
	start int
	// end 是最后一个非空白字符之后的字节下标。
	end int
}

// ChunkText 按自然段边界和字符阈值把纯文本切成有序片段。
//
// 算法只做确定性的内存计算：它不会生成数据库 ID、不会调用 Embedding，也不会写数据库。
// 返回的 Chunk 会在 ProcessDocument 中补齐知识库、文档和 ID 后再进行持久化校验。
func ChunkText(text string, options domain.ChunkOptions) ([]domain.Chunk, error) {
	// 配置的默认值和范围校验由领域类型统一负责，避免算法内部悄悄接受相互矛盾的阈值。
	normalized, err := options.Normalize()
	// 配置非法时不尝试扫描文本，直接把领域参数错误返回调用方。
	if err != nil {
		return nil, err
	}

	// 纯空白内容没有任何可用于检索的信息，返回空切片让应用用例把处理标记为失败。
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}

	// 先识别自然段，再把单个超长自然段拆成不超过 MaxChars 的小块。
	// 这一步补上旧实现对“整篇没有空行”的长文本无法限制块大小的问题。
	blocks := splitOversizedBlocks(text, textBlocks(text), normalized.MaxChars)
	// 理论上非空白文本应产生 block；兜底分支保证极端输入仍能形成一个完整分块。
	if len(blocks) == 0 {
		return []domain.Chunk{newChunk(0, text, 0, len(text))}, nil
	}

	// 按自然段数量预估容量，减少 append 过程中切片扩容次数。
	chunks := make([]domain.Chunk, 0, len(blocks))
	// 当前累计分块从第一个自然段起点开始。
	chunkStart := blocks[0].start
	// 初始结束位置是第一个自然段的结束位置。
	chunkEnd := blocks[0].end

	// 从第二个自然段开始尝试合并，因为第一个已经作为当前累计区间。
	for _, block := range blocks[1:] {
		// currentLength 表示当前已经累计内容的字节长度。
		currentLength := chunkEnd - chunkStart
		// candidateLength 表示把下一自然段也合并后会达到的总字节长度。
		candidateLength := block.end - chunkStart

		// 两种情况必须先结束当前块：
		// 1. 当前块已达到最小值，再加下一段会超过目标值；
		// 2. 无论最小值如何，再加下一段都会突破硬上限。
		if (currentLength >= normalized.MinChars && candidateLength > normalized.TargetChars) || candidateLength > normalized.MaxChars {
			// 当前区间已经应该结束时，按现有顺序号创建一个临时领域 Chunk。
			chunks = append(chunks, newChunk(len(chunks), text[chunkStart:chunkEnd], chunkStart, chunkEnd))
			// 下一块从当前自然段起点重新累计。
			chunkStart = block.start
		}
		// 无论是否刚提交上一块，当前自然段都成为累计区间的新结束位置。
		chunkEnd = block.end
	}

	// 循环只会在看到“下一块”时提交前一块，因此最后一个累计区间需要在循环后补交。
	if chunkStart < chunkEnd {
		chunks = append(chunks, newChunk(len(chunks), text[chunkStart:chunkEnd], chunkStart, chunkEnd))
	}
	// 返回顺序稳定的分块切片，后续应用层会补齐 ID 和归属信息。
	return chunks, nil
}

// textBlocks 扫描空行分隔的自然段，并去掉每段首尾的行空白。
func textBlocks(text string) []textBlock {
	// blocks 按扫描顺序保存自然段，不做额外排序。
	var blocks []textBlock
	// start=-1 表示当前还没有进入一个包含非空白字符的自然段。
	start := -1
	// lastNonSpaceEnd 记录最近非空白字符结束位置，用于裁掉段尾空格和换行。
	lastNonSpaceEnd := 0

	// range 返回的是每个 Unicode 字符起始处的字节下标，正好能用于后续字符串切片。
	for index, currentRune := range text {
		// 遇到当前自然段的第一个非行空白字符时记录起点。
		if !isLineSpace(currentRune) && start == -1 {
			start = index
		}
		// 自然段进行中时，每个非空白字符都会推进有效结束位置。
		if start != -1 && !isLineSpace(currentRune) {
			lastNonSpaceEnd = index + utf8.RuneLen(currentRune)
		}
		// 当前换行后还存在第二个换行时，说明一个自然段已经完整结束。
		if currentRune == '\n' && start != -1 && nextIsParagraphBreak(text, index+1) {
			// 只保存有效内容区间，不把分隔空行包含进分块。
			blocks = append(blocks, textBlock{start: start, end: lastNonSpaceEnd})
			// 重置起点，继续寻找下一个自然段的首字符。
			start = -1
		}
	}

	// 文件末尾不一定带空行，仍在进行中的最后自然段要在循环后补交。
	if start != -1 {
		blocks = append(blocks, textBlock{start: start, end: lastNonSpaceEnd})
	}
	// 返回按原文顺序排列的自然段区间。
	return blocks
}

// splitOversizedBlocks 把大于硬上限的单个自然段切成多个连续区间。
func splitOversizedBlocks(text string, blocks []textBlock, maxBytes int) []textBlock {
	// 最坏情况下每个输入自然段至少产生一个输出，因此用输入长度作为初始容量。
	result := make([]textBlock, 0, len(blocks))
	// 逐个自然段处理，保持原始顺序不变。
	for _, block := range blocks {
		// start 表示当前自然段尚未输出部分的起点。
		start := block.start
		// 只要剩余区间仍超过硬上限，就继续切出一块。
		for block.end-start > maxBytes {
			// 先按字节预算计算候选结束位置。
			next := start + maxBytes
			// 如果预算末尾落在 UTF-8 多字节字符中间，就向前退到一个合法字符边界，避免产生损坏字符串。
			for next > start && !utf8.RuneStart(text[next]) {
				// 每次向前退一个字节，直到落到 UTF-8 字符起始位置。
				next--
			}
			// 极小阈值可能连一个中文字符都装不下；此时至少完整放入一个字符，保证循环能够前进。
			if next == start {
				// 解码第一个完整字符，获取它实际占用的字节数。
				_, size := utf8.DecodeRuneInString(text[start:block.end])
				// 即使字符大于配置上限，也至少完整输出该字符，不能切出非法 UTF-8。
				next = start + size
			}
			// 保存本次切出的连续区间。
			result = append(result, textBlock{start: start, end: next})
			// 下一轮从刚刚输出的结束位置继续，保证没有重叠也没有遗漏。
			start = next
		}
		// 循环结束后仍有不足上限的尾部时，把尾部作为最后一块保存。
		if start < block.end {
			result = append(result, textBlock{start: start, end: block.end})
		}
	}
	// 返回所有已按硬上限拆分的自然段区间。
	return result
}

// isLineSpace 只识别段落扫描需要忽略的常见行空白。
func isLineSpace(currentRune rune) bool {
	// 这里只处理段落结构所需的四种常见空白，不承担完整 Unicode 空白分类。
	return currentRune == ' ' || currentRune == '\t' || currentRune == '\r' || currentRune == '\n'
}

// nextIsParagraphBreak 判断当前换行之后是否还有第二个换行，中间允许出现空格、制表符和回车。
func nextIsParagraphBreak(text string, offset int) bool {
	// 从当前换行的下一个字节开始向后查看，直到确认第二个换行或遇到正文。
	for offset < len(text) {
		// 这里检查的是 ASCII 空白字节，不会落入中文多字节字符内部，因为只在换行后顺序前进。
		switch text[offset] {
		case ' ', '\t', '\r':
			// 空格、制表符和回车可以出现在两个换行之间，继续向后看。
			offset++
		case '\n':
			// 找到第二个换行，当前边界就是自然段分隔。
			return true
		default:
			// 在第二个换行前遇到正文，说明只是普通行内换行。
			return false
		}
	}
	// 扫描到文本末尾仍没有第二个换行，不视为额外段落分隔。
	return false
}

// newChunk 创建算法阶段的临时 Chunk。
// ID、KbID、DocumentID 和 TokenCount 依赖应用流程上下文，会在 ProcessDocument 中统一补齐。
func newChunk(index int, content string, start int, end int) domain.Chunk {
	// 只填充算法现在能够确定的字段，依赖业务上下文的字段留给 ProcessDocument。
	return domain.Chunk{
		Index:         index,
		Content:       content,
		CharCount:     utf8.RuneCountInString(content),
		StartPosition: start,
		EndPosition:   end,
		Metadata:      map[string]any{},
	}
}
