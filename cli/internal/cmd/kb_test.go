package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vmarinogg/leo-core/cli/internal/adapters/storage"
)

// setupTestKB creates a .leo/ with a JSONAdapter and returns the temp dir.
func setupTestKB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	leoDir := filepath.Join(dir, ".leo")
	os.MkdirAll(filepath.Join(leoDir, "memory"), 0755)

	// Write empty index.
	idx := map[string]any{
		"version": "1", "last_rebuilt": "", "by_tag": map[string]any{},
		"by_type": map[string]any{}, "by_scope": map[string]any{},
		"by_lifecycle": map[string]any{},
	}
	data, _ := json.MarshalIndent(idx, "", "  ")
	os.WriteFile(filepath.Join(leoDir, "index.json"), data, 0644)

	return dir
}

func writeTestDoc(t *testing.T, dir string, doc *storage.Doc) {
	t.Helper()
	adapter := storage.NewJSONAdapter(filepath.Join(dir, ".leo"))
	if err := adapter.Write(doc); err != nil {
		t.Fatalf("writing test doc: %v", err)
	}
}

func sampleDoc(id string) *storage.Doc {
	return &storage.Doc{
		ID: id, Type: "fact", Lifecycle: "state", Scope: "project",
		Tags: []string{"test"}, Created: time.Now().UTC(), CreatedBy: "test",
		Updated: time.Now().UTC(), UpdatedBy: "test",
		Content: map[string]any{"fact": "sample fact"},
	}
}

func TestValidateCmd_AllValid(t *testing.T) {
	dir := setupTestKB(t)
	writeTestDoc(t, dir, sampleDoc("valid-doc"))

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"validate", "--all"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("validate --all failed: %v", err)
	}

	if !strings.Contains(buf.String(), "All documents valid") {
		t.Errorf("expected success message, got: %s", buf.String())
	}
}

func TestReindexCmd_RebuildsIndex(t *testing.T) {
	dir := setupTestKB(t)
	writeTestDoc(t, dir, sampleDoc("reindex-doc"))

	// Corrupt the index.
	indexPath := filepath.Join(dir, ".leo", "kb", "index.json")
	os.WriteFile(indexPath, []byte(`{}`), 0644)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"reindex"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("reindex failed: %v", err)
	}

	if !strings.Contains(buf.String(), "Index rebuilt") {
		t.Errorf("expected rebuild message, got: %s", buf.String())
	}
}
