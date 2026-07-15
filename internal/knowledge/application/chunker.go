package application

import (
	"strings"
	"unicode/utf8"

	"github.com/luqingjia/TravelAgent/internal/knowledge/domain"
)

// textBlock 记录一个自然段在原始字符串中的半开区间 [start, end)。
// 使用字节下标是因为 Go 字符串切片按字节工作，最终可以无损地从原文恢复 Content。
type textBlock struct {
	start int
	end   int
}

// ChunkText 按自然段边界和字符阈值把纯文本切成有序片段。
//
// 算法只做确定性的内存计算：它不会生成数据库 ID、不会调用 Embedding，也不会写数据库。
// 返回的 Chunk 会在 ProcessDocument 中补齐知识库、文档和 ID 后再进行持久化校验。
func ChunkText(text string, options domain.ChunkOptions) ([]domain.Chunk, error) {
	// 配置的默认值和范围校验由领域类型统一负责，避免算法内部悄悄接受相互矛盾的阈值。
	normalized, err := options.Normalize()
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
	if len(blocks) == 0 {
		return []domain.Chunk{newChunk(0, text, 0, len(text))}, nil
	}

	chunks := make([]domain.Chunk, 0, len(blocks))
	chunkStart := blocks[0].start
	chunkEnd := blocks[0].end

	for _, block := range blocks[1:] {
		currentLength := chunkEnd - chunkStart
		candidateLength := block.end - chunkStart

		// 两种情况必须先结束当前块：
		// 1. 当前块已达到最小值，再加下一段会超过目标值；
		// 2. 无论最小值如何，再加下一段都会突破硬上限。
		if (currentLength >= normalized.MinChars && candidateLength > normalized.TargetChars) || candidateLength > normalized.MaxChars {
			chunks = append(chunks, newChunk(len(chunks), text[chunkStart:chunkEnd], chunkStart, chunkEnd))
			chunkStart = block.start
		}
		chunkEnd = block.end
	}

	// 循环只会在看到“下一块”时提交前一块，因此最后一个累计区间需要在循环后补交。
	if chunkStart < chunkEnd {
		chunks = append(chunks, newChunk(len(chunks), text[chunkStart:chunkEnd], chunkStart, chunkEnd))
	}
	return chunks, nil
}

// textBlocks 扫描空行分隔的自然段，并去掉每段首尾的行空白。
func textBlocks(text string) []textBlock {
	var blocks []textBlock
	start := -1
	lastNonSpaceEnd := 0

	// range 返回的是每个 Unicode 字符起始处的字节下标，正好能用于后续字符串切片。
	for index, currentRune := range text {
		if !isLineSpace(currentRune) && start == -1 {
			start = index
		}
		if start != -1 && !isLineSpace(currentRune) {
			lastNonSpaceEnd = index + utf8.RuneLen(currentRune)
		}
		if currentRune == '\n' && start != -1 && nextIsParagraphBreak(text, index+1) {
			blocks = append(blocks, textBlock{start: start, end: lastNonSpaceEnd})
			start = -1
		}
	}

	if start != -1 {
		blocks = append(blocks, textBlock{start: start, end: lastNonSpaceEnd})
	}
	return blocks
}

// splitOversizedBlocks 把大于硬上限的单个自然段切成多个连续区间。
func splitOversizedBlocks(text string, blocks []textBlock, maxBytes int) []textBlock {
	result := make([]textBlock, 0, len(blocks))
	for _, block := range blocks {
		start := block.start
		for block.end-start > maxBytes {
			next := start + maxBytes
			// 如果预算末尾落在 UTF-8 多字节字符中间，就向前退到一个合法字符边界，避免产生损坏字符串。
			for next > start && !utf8.RuneStart(text[next]) {
				next--
			}
			// 极小阈值可能连一个中文字符都装不下；此时至少完整放入一个字符，保证循环能够前进。
			if next == start {
				_, size := utf8.DecodeRuneInString(text[start:block.end])
				next = start + size
			}
			result = append(result, textBlock{start: start, end: next})
			start = next
		}
		if start < block.end {
			result = append(result, textBlock{start: start, end: block.end})
		}
	}
	return result
}

// isLineSpace 只识别段落扫描需要忽略的常见行空白。
func isLineSpace(currentRune rune) bool {
	return currentRune == ' ' || currentRune == '\t' || currentRune == '\r' || currentRune == '\n'
}

// nextIsParagraphBreak 判断当前换行之后是否还有第二个换行，中间允许出现空格、制表符和回车。
func nextIsParagraphBreak(text string, offset int) bool {
	for offset < len(text) {
		switch text[offset] {
		case ' ', '\t', '\r':
			offset++
		case '\n':
			return true
		default:
			return false
		}
	}
	return false
}

// newChunk 创建算法阶段的临时 Chunk。
// ID、KbID、DocumentID 和 TokenCount 依赖应用流程上下文，会在 ProcessDocument 中统一补齐。
func newChunk(index int, content string, start int, end int) domain.Chunk {
	return domain.Chunk{
		Index:         index,
		Content:       content,
		CharCount:     utf8.RuneCountInString(content),
		StartPosition: start,
		EndPosition:   end,
		Metadata:      map[string]any{},
	}
}
