package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/momhq/mom/cli/internal/adapters/storage"
)

// setupTestMemory creates a .mom/ with a JSONAdapter and returns the temp dir.
func setupTestMemory(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	leoDir := filepath.Join(dir, ".mom")
	os.MkdirAll(filepath.Join(leoDir, "memory"), 0755)

	return dir
}

func writeTestDoc(t *testing.T, dir string, doc *storage.Doc) {
	t.Helper()
	adapter := storage.NewJSONAdapter(filepath.Join(dir, ".mom"))
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
	dir := setupTestMemory(t)
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

