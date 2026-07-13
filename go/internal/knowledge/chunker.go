package knowledge

import (
	"strings"
	"unicode/utf8"
)

func ChunkText(text string, options ChunkOptions) []Chunk {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	options = normalizeChunkOptions(options)
	blocks := textBlocks(text)
	if len(blocks) == 0 {
		return []Chunk{newChunk(0, text, 0, len(text))}
	}

	var chunks []Chunk
	chunkStart := blocks[0].start
	chunkEnd := blocks[0].end
	for i, block := range blocks {
		if i == 0 {
			continue
		}
		nextLen := block.end - chunkStart
		currentLen := chunkEnd - chunkStart
		if currentLen >= options.MinChars && nextLen > options.TargetChars {
			chunks = append(chunks, newChunk(len(chunks), text[chunkStart:chunkEnd], chunkStart, chunkEnd))
			chunkStart = block.start
		}
		chunkEnd = block.end
		if chunkEnd-chunkStart >= options.MaxChars {
			chunks = append(chunks, newChunk(len(chunks), text[chunkStart:chunkEnd], chunkStart, chunkEnd))
			if i+1 < len(blocks) {
				chunkStart = blocks[i+1].start
				chunkEnd = blocks[i+1].end
			}
		}
	}
	if chunkStart < chunkEnd {
		chunks = append(chunks, newChunk(len(chunks), text[chunkStart:chunkEnd], chunkStart, chunkEnd))
	}
	return chunks
}

func normalizeChunkOptions(options ChunkOptions) ChunkOptions {
	if options.TargetChars <= 0 {
		options.TargetChars = 800
	}
	if options.MaxChars <= 0 {
		options.MaxChars = 1200
	}
	if options.MinChars <= 0 {
		options.MinChars = 100
	}
	if options.MaxChars < options.TargetChars {
		options.MaxChars = options.TargetChars
	}
	return options
}

type textBlock struct {
	start int
	end   int
}

func textBlocks(text string) []textBlock {
	var blocks []textBlock
	start := -1
	lastNonSpaceEnd := 0
	for index, r := range text {
		if !isLineSpace(r) && start == -1 {
			start = index
		}
		if start != -1 && !isLineSpace(r) {
			lastNonSpaceEnd = index + utf8.RuneLen(r)
		}
		if r == '\n' && start != -1 {
			if nextIsParagraphBreak(text, index+1) {
				blocks = append(blocks, textBlock{start: start, end: lastNonSpaceEnd})
				start = -1
			}
		}
	}
	if start != -1 {
		blocks = append(blocks, textBlock{start: start, end: lastNonSpaceEnd})
	}
	return blocks
}

func isLineSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\n'
}

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

func newChunk(index int, content string, start int, end int) Chunk {
	return Chunk{
		Index:         index,
		Content:       content,
		CharCount:     utf8.RuneCountInString(content),
		StartPosition: start,
		EndPosition:   end,
		Metadata:      map[string]any{},
	}
}
