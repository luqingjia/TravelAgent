package embedding

import "testing"

func TestVectorTextFormatsPgvectorLiteral(t *testing.T) {
	got, err := VectorText([]float32{0.1, -2, 3.25})
	if err != nil {
		t.Fatalf("VectorText returned error: %v", err)
	}
	if got != "[0.1,-2,3.25]" {
		t.Fatalf("VectorText = %q, want [0.1,-2,3.25]", got)
	}
}

func TestValidateDimensionsRejectsWrongLength(t *testing.T) {
	vector := make([]float32, 1535)
	if err := ValidateDimensions(vector, 1536); err == nil {
		t.Fatalf("ValidateDimensions should reject 1535 dimensions")
	}
}

func TestFakeEmbedderReturnsConfiguredDimensions(t *testing.T) {
	embedder := NewFakeEmbedder(1536)
	vectors, err := embedder.EmbedTexts(nil, []string{"hello", "world"})
	if err != nil {
		t.Fatalf("EmbedTexts returned error: %v", err)
	}
	if len(vectors) != 2 {
		t.Fatalf("vectors len = %d, want 2", len(vectors))
	}
	for i, vector := range vectors {
		if len(vector) != 1536 {
			t.Fatalf("vector %d dimensions = %d, want 1536", i, len(vector))
		}
	}
}
