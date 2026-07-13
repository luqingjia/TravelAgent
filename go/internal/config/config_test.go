package config

import "testing"

func TestLoadUsesDefaultsForMVP(t *testing.T) {
	t.Setenv("GO_AGENT_PORT", "")
	t.Setenv("EMBEDDING_DIMENSIONS", "")
	t.Setenv("KNOWLEDGE_DOCUMENT_ALLOWED_EXTENSIONS", "")
	t.Setenv("KNOWLEDGE_DOCUMENT_MAX_SIZE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Port != "8081" {
		t.Fatalf("default port = %q, want 8081", cfg.Port)
	}
	if cfg.Embedding.Dimensions != 1536 {
		t.Fatalf("default embedding dimensions = %d, want 1536", cfg.Embedding.Dimensions)
	}
	if cfg.Document.MaxUploadBytes != 50*1024*1024 {
		t.Fatalf("default max upload bytes = %d, want 50MiB", cfg.Document.MaxUploadBytes)
	}
	if !cfg.Document.IsExtensionAllowed("md") || !cfg.Document.IsExtensionAllowed(".markdown") {
		t.Fatalf("default allowed extensions should include md and markdown")
	}
}

func TestLoadParsesEnvironmentOverrides(t *testing.T) {
	t.Setenv("GO_AGENT_PORT", "9090")
	t.Setenv("POSTGRESQL_DSN", "postgres://user:pass@localhost:5432/kenagent?sslmode=disable")
	t.Setenv("EMBEDDING_DIMENSIONS", "1024")
	t.Setenv("KNOWLEDGE_DOCUMENT_ALLOWED_EXTENSIONS", "txt,md")
	t.Setenv("KNOWLEDGE_DOCUMENT_MAX_SIZE", "2KB")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Port != "9090" {
		t.Fatalf("port = %q, want 9090", cfg.Port)
	}
	if cfg.Database.DSN == "" {
		t.Fatalf("database DSN should come from POSTGRESQL_DSN")
	}
	if cfg.Embedding.Dimensions != 1024 {
		t.Fatalf("embedding dimensions = %d, want 1024", cfg.Embedding.Dimensions)
	}
	if cfg.Document.MaxUploadBytes != 2*1024 {
		t.Fatalf("max upload bytes = %d, want 2048", cfg.Document.MaxUploadBytes)
	}
	if cfg.Document.IsExtensionAllowed("pdf") {
		t.Fatalf("pdf should not be allowed after override")
	}
}
