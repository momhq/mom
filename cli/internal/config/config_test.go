package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault_HasSaneValues(t *testing.T) {
	cfg := Default()

	if cfg.Version != "1" {
		t.Errorf("expected version %q, got %q", "1", cfg.Version)
	}
	if cfg.Runtime != "claude" {
		t.Errorf("expected runtime %q, got %q", "claude", cfg.Runtime)
	}
	if cfg.Owner.Mode != "concise" {
		t.Errorf("expected mode %q, got %q", "concise", cfg.Owner.Mode)
	}
	if cfg.Owner.DefaultProfile != "generalist" {
		t.Errorf("expected default profile %q, got %q", "generalist", cfg.Owner.DefaultProfile)
	}
	if cfg.Specialists.DefaultModel != "sonnet" {
		t.Errorf("expected default model %q, got %q", "sonnet", cfg.Specialists.DefaultModel)
	}
	if !cfg.KB.AutoPropagate {
		t.Error("expected auto_propagate to be true")
	}
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()

	original := Default()
	original.Runtime = "cursor"
	original.Owner.Language = "pt-BR"

	if err := Save(dir, &original); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists.
	path := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config.yaml not created: %v", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Runtime != "cursor" {
		t.Errorf("expected runtime %q, got %q", "cursor", loaded.Runtime)
	}
	if loaded.Owner.Language != "pt-BR" {
		t.Errorf("expected language %q, got %q", "pt-BR", loaded.Owner.Language)
	}
	if loaded.Version != original.Version {
		t.Errorf("version mismatch: %q vs %q", original.Version, loaded.Version)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	if _, err := Load("/nonexistent/dir"); err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(":\n  :\n    - :\n  ]["), 0644)

	if _, err := Load(dir); err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLeoDir(t *testing.T) {
	got := LeoDir("/home/user/project")
	expected := filepath.Join("/home/user/project", ".leo")
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}
