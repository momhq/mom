package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testDoc(id string) *Doc {
	return &Doc{
		ID:        id,
		Type:      "fact",
		Lifecycle: "state",
		Scope:     "project",
		Tags:      []string{"test"},
		Created:   time.Now().UTC(),
		CreatedBy: "test",
		Updated:   time.Now().UTC(),
		UpdatedBy: "test",
		Content:   map[string]any{"fact": "test fact"},
	}
}

func setupAdapter(t *testing.T) (*JSONAdapter, string) {
	t.Helper()
	dir := t.TempDir()
	leoDir := filepath.Join(dir, ".leo")
	os.MkdirAll(filepath.Join(leoDir, "kb", "docs"), 0755)
	return NewJSONAdapter(leoDir), leoDir
}

func TestJSONAdapter_WriteAndRead(t *testing.T) {
	adapter, _ := setupAdapter(t)
	doc := testDoc("test-doc")

	if err := adapter.Write(doc); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	got, err := adapter.Read("test-doc")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if got.ID != "test-doc" {
		t.Errorf("expected id %q, got %q", "test-doc", got.ID)
	}
	if got.Type != "fact" {
		t.Errorf("expected type %q, got %q", "fact", got.Type)
	}
}

func TestJSONAdapter_WriteValidation(t *testing.T) {
	adapter, _ := setupAdapter(t)
	doc := &Doc{
		ID:   "INVALID_ID",
		Type: "fact",
	}

	if err := adapter.Write(doc); err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

func TestJSONAdapter_Delete(t *testing.T) {
	adapter, _ := setupAdapter(t)
	doc := testDoc("to-delete")

	adapter.Write(doc)

	if err := adapter.Delete("to-delete"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if _, err := adapter.Read("to-delete"); err == nil {
		t.Fatal("expected error reading deleted doc, got nil")
	}
}

func TestJSONAdapter_Query(t *testing.T) {
	adapter, _ := setupAdapter(t)

	adapter.Write(testDoc("doc-a"))

	docB := testDoc("doc-b")
	docB.Type = "rule"
	docB.Tags = []string{"test"}
	docB.Content = map[string]any{
		"rule": "test rule", "why": "test", "how_to_apply": []any{"test"},
		"responsibility": "test",
	}
	adapter.Write(docB)

	docs, err := adapter.Query(QueryFilter{Type: "fact"})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
	if docs[0].ID != "doc-a" {
		t.Errorf("expected doc-a, got %s", docs[0].ID)
	}
}

func TestJSONAdapter_RebuildIndex(t *testing.T) {
	adapter, _ := setupAdapter(t)
	adapter.Write(testDoc("indexed-doc"))

	idx, err := adapter.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if ids, ok := idx.ByType["fact"]; !ok || len(ids) == 0 {
		t.Fatal("expected fact type in index")
	}
}

func TestJSONAdapter_BulkWrite(t *testing.T) {
	adapter, _ := setupAdapter(t)

	docs := []*Doc{testDoc("bulk-a"), testDoc("bulk-b"), testDoc("bulk-c")}
	if err := adapter.BulkWrite(docs); err != nil {
		t.Fatalf("BulkWrite failed: %v", err)
	}

	idx, err := adapter.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if ids := idx.ByType["fact"]; len(ids) != 3 {
		t.Fatalf("expected 3 docs in index, got %d", len(ids))
	}
}

func TestJSONAdapter_Health(t *testing.T) {
	adapter, _ := setupAdapter(t)

	status, err := adapter.Health()
	if err != nil {
		t.Fatalf("Health failed: %v", err)
	}
	if !status.OK {
		t.Fatalf("expected healthy, got: %s", status.Message)
	}
}
