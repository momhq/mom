package storage_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vmarinogg/leo-core/cli/internal/adapters/storage"
	"github.com/vmarinogg/leo-core/cli/internal/kb"
)

// makeScope creates a .leo/ with config.yaml and an optional memory dir.
func makeScope(t *testing.T, parent, scopeLabel string) string {
	t.Helper()
	leoDir := filepath.Join(parent, ".mom")
	if err := os.MkdirAll(filepath.Join(leoDir, "memory"), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := "version: \"1\"\nscope: " + scopeLabel + "\nruntimes:\n  claude:\n    enabled: true\n"
	if err := os.WriteFile(filepath.Join(leoDir, "config.yaml"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	return leoDir
}

// writeDoc writes a minimal valid KB JSON doc to leoDir/memory/<id>.json.
func writeDoc(t *testing.T, leoDir, id string) {
	t.Helper()
	doc := &kb.Doc{
		ID:        id,
		Type:      "fact",
		Lifecycle: "permanent",
		Scope:     "project",
		Tags:      []string{"test"},
		Created:   time.Now().UTC(),
		CreatedBy: "test",
		Updated:   time.Now().UTC(),
		UpdatedBy: "test",
		Content:   map[string]any{"body": "body"},
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(leoDir, "memory", id+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestReadAllScopes_NoScopes(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)

	docs, err := storage.ReadAllScopes(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(docs) != 0 {
		t.Errorf("expected 0 docs, got %d", len(docs))
	}
}

func TestReadAllScopes_SingleScope(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)

	leoDir := makeScope(t, root, "repo")
	writeDoc(t, leoDir, "my-fact")

	docs, err := storage.ReadAllScopes(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
	if docs[0].ID != "my-fact" {
		t.Errorf("doc ID = %q", docs[0].ID)
	}
	if docs[0].Inherited {
		t.Error("nearest scope doc should not be marked inherited")
	}
}

func TestReadAllScopes_MultiScope_ChildWins(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", root)

	leoUser := makeScope(t, root, "user")
	leoRepo := makeScope(t, sub, "repo")

	// Same ID in both scopes — child copy (repo) wins.
	writeDoc(t, leoUser, "shared-fact")
	writeDoc(t, leoRepo, "shared-fact")
	// Unique doc only in user scope.
	writeDoc(t, leoUser, "user-only")
	// Unique doc only in repo scope.
	writeDoc(t, leoRepo, "repo-only")

	docs, err := storage.ReadAllScopes(sub)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byID := make(map[string]*storage.InheritedDoc)
	for _, d := range docs {
		byID[d.ID] = d
	}

	if _, ok := byID["shared-fact"]; !ok {
		t.Fatal("shared-fact not found")
	}
	if byID["shared-fact"].Inherited {
		t.Error("shared-fact should come from child (not inherited)")
	}
	if byID["shared-fact"].ScopeLabel != "repo" {
		t.Errorf("shared-fact.ScopeLabel = %q, want repo", byID["shared-fact"].ScopeLabel)
	}

	if _, ok := byID["user-only"]; !ok {
		t.Fatal("user-only not found")
	}
	if !byID["user-only"].Inherited {
		t.Error("user-only should be marked inherited")
	}

	if _, ok := byID["repo-only"]; !ok {
		t.Fatal("repo-only not found")
	}
	if byID["repo-only"].Inherited {
		t.Error("repo-only should not be inherited")
	}

	// Total: 3 unique docs (shared-fact, user-only, repo-only).
	if len(docs) != 3 {
		t.Errorf("expected 3 docs, got %d", len(docs))
	}
}

func TestReadAllScopes_InheritedReadOnly(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	os.MkdirAll(sub, 0755) //nolint:errcheck
	t.Setenv("HOME", root)

	leoUser := makeScope(t, root, "user")
	makeScope(t, sub, "repo") // repo scope has no docs

	writeDoc(t, leoUser, "ancestor-fact")

	docs, err := storage.ReadAllScopes(sub)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
	if !docs[0].Inherited {
		t.Error("doc from ancestor scope must be marked Inherited=true")
	}
	if docs[0].ScopeLabel != "user" {
		t.Errorf("ScopeLabel = %q, want user", docs[0].ScopeLabel)
	}
}
