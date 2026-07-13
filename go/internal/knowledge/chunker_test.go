package knowledge

import "testing"

func TestChunkTextKeepsOrderAndPositions(t *testing.T) {
	text := "第一段内容，适合放在第一个块。\n\n第二段内容会继续被切分。\n\n第三段内容用于确认顺序。"

	chunks := ChunkText(text, ChunkOptions{
		MinChars:    8,
		TargetChars: 24,
		MaxChars:    48,
	})
	if len(chunks) < 2 {
		t.Fatalf("chunk count = %d, want at least 2", len(chunks))
	}
	for i, chunk := range chunks {
		if chunk.Index != i {
			t.Fatalf("chunk index = %d, want %d", chunk.Index, i)
		}
		if chunk.StartPosition < 0 || chunk.EndPosition <= chunk.StartPosition {
			t.Fatalf("invalid positions for chunk %d: %+v", i, chunk)
		}
		if got := text[chunk.StartPosition:chunk.EndPosition]; got != chunk.Content {
			t.Fatalf("chunk %d content does not match original substring", i)
		}
	}
}

func TestChunkTextRejectsBlankText(t *testing.T) {
	chunks := ChunkText(" \n\t ", ChunkOptions{TargetChars: 20, MaxChars: 40})
	if len(chunks) != 0 {
		t.Fatalf("blank text chunks = %d, want 0", len(chunks))
	}
}
