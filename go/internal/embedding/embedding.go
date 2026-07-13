package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Embedder interface {
	EmbedTexts(ctx context.Context, texts []string) ([][]float32, error)
}

type ClientConfig struct {
	APIKey     string
	BaseURL    string
	Model      string
	Dimensions int
}

type Client struct {
	config     ClientConfig
	httpClient *http.Client
}

func NewClient(config ClientConfig) *Client {
	return &Client{
		config: config,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *Client) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if strings.TrimSpace(c.config.APIKey) == "" {
		return nil, fmt.Errorf("embedding api key is empty")
	}
	body, err := json.Marshal(embeddingRequest{
		Model:      c.config.Model,
		Input:      texts,
		Dimensions: c.config.Dimensions,
	})
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(c.config.BaseURL, "/") + "/v1/embeddings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embedding api returned status %d", resp.StatusCode)
	}

	var decoded embeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	vectors := make([][]float32, len(decoded.Data))
	for _, item := range decoded.Data {
		if item.Index < 0 || item.Index >= len(vectors) {
			return nil, fmt.Errorf("embedding response index %d out of range", item.Index)
		}
		vectors[item.Index] = item.Embedding
	}
	if len(vectors) != len(texts) {
		return nil, fmt.Errorf("embedding response count %d does not match input count %d", len(vectors), len(texts))
	}
	for i, vector := range vectors {
		if len(vector) == 0 {
			return nil, fmt.Errorf("embedding response missing vector at index %d", i)
		}
	}
	return vectors, nil
}

type embeddingRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions int      `json:"dimensions,omitempty"`
}

type embeddingResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

type FakeEmbedder struct {
	dimensions int
}

func NewFakeEmbedder(dimensions int) FakeEmbedder {
	return FakeEmbedder{dimensions: dimensions}
}

func (f FakeEmbedder) EmbedTexts(_ context.Context, texts []string) ([][]float32, error) {
	return fakeVectors(f.dimensions, texts), nil
}

func fakeVectors(dimensions int, texts []string) [][]float32 {
	vectors := make([][]float32, len(texts))
	for i, text := range texts {
		vector := make([]float32, dimensions)
		seed := float32(len(text) + i + 1)
		for j := range vector {
			vector[j] = seed / float32(j+1+dimensions)
		}
		vectors[i] = vector
	}
	return vectors
}

func ValidateDimensions(vector []float32, dimensions int) error {
	if len(vector) != dimensions {
		return fmt.Errorf("embedding dimensions = %d, want %d", len(vector), dimensions)
	}
	return nil
}

func VectorText(vector []float32) (string, error) {
	if len(vector) == 0 {
		return "", fmt.Errorf("embedding vector is empty")
	}
	parts := make([]string, len(vector))
	for i, value := range vector {
		parts[i] = strconv.FormatFloat(float64(value), 'f', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]", nil
}
