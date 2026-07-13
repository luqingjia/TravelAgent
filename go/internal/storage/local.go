package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"travel-agent-go/internal/knowledge"
)

type LocalStorage struct {
	baseDir string
}

func NewLocal(baseDir string) *LocalStorage {
	return &LocalStorage{baseDir: baseDir}
}

func (s *LocalStorage) Put(_ context.Context, input knowledge.StoredObjectInput) (knowledge.StoredObject, error) {
	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return knowledge.StoredObject{}, err
	}
	name := fmt.Sprintf("%d-%s", time.Now().UnixNano(), filepath.Base(input.FileName))
	path := filepath.Join(s.baseDir, name)
	if err := os.WriteFile(path, input.Content, 0o644); err != nil {
		return knowledge.StoredObject{}, err
	}
	return knowledge.StoredObject{
		URI:         "local://" + filepath.ToSlash(path),
		FileName:    input.FileName,
		ContentType: input.ContentType,
		Size:        int64(len(input.Content)),
	}, nil
}

func (s *LocalStorage) Get(_ context.Context, uri string) ([]byte, error) {
	path := strings.TrimPrefix(uri, "local://")
	return os.ReadFile(filepath.FromSlash(path))
}

func (s *LocalStorage) Delete(_ context.Context, uri string) error {
	path := strings.TrimPrefix(uri, "local://")
	if err := os.Remove(filepath.FromSlash(path)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
