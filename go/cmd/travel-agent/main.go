package main

import (
	"context"
	"log"

	"travel-agent-go/internal/config"
	"travel-agent-go/internal/database"
	"travel-agent-go/internal/embedding"
	httpapi "travel-agent-go/internal/http"
	"travel-agent-go/internal/knowledge"
	"travel-agent-go/internal/storage"
)

func main() {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	db, err := database.Open(ctx, cfg.Database.DSN)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	repo := knowledge.NewSQLRepository(db)
	store, err := buildStorage(ctx, cfg)
	if err != nil {
		log.Fatalf("create storage: %v", err)
	}
	embedder := embedding.NewClient(embedding.ClientConfig{
		APIKey:     cfg.Embedding.APIKey,
		BaseURL:    cfg.Embedding.BaseURL,
		Model:      cfg.Embedding.Model,
		Dimensions: cfg.Embedding.Dimensions,
	})
	service := knowledge.NewService(repo, store, embedder, cfg.Document)
	router := httpapi.NewRouter(service)
	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatalf("run server: %v", err)
	}
}

func buildStorage(ctx context.Context, cfg config.Config) (knowledge.Storage, error) {
	if cfg.Storage.S3Enabled {
		return storage.NewS3(ctx, cfg.Storage)
	}
	return storage.NewLocal(cfg.Storage.LocalDir), nil
}
