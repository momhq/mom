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
	os.MkdirAll(filepath.Join(leoDir, "kb", "docs"), 0755)

	// Write empty index.
	idx := map[string]any{
		"version": "1", "last_rebuilt": "", "by_tag": map[string]any{},
		"by_type": map[string]any{}, "by_scope": map[string]any{},
		"by_lifecycle": map[string]any{},
	}
	data, _ := json.MarshalIndent(idx, "", "  ")
	os.WriteFile(filepath.Join(leoDir, "kb", "index.json"), data, 0644)

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

func TestReadCmd_ReturnsDoc(t *testing.T) {
	dir := setupTestKB(t)
	writeTestDoc(t, dir, sampleDoc("read-target"))

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"read", "read-target"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if !strings.Contains(buf.String(), "read-target") {
		t.Errorf("output should contain doc id, got: %s", buf.String())
	}
}

func TestReadCmd_NotFound(t *testing.T) {
	dir := setupTestKB(t)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"read", "nonexistent"})

	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error for missing doc")
	}
}

func TestWriteCmd_WritesAndRebuildsIndex(t *testing.T) {
	dir := setupTestKB(t)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Create a doc file to write.
	docFile := filepath.Join(dir, "new-doc.json")
	doc := sampleDoc("written-doc")
	data, _ := json.MarshalIndent(doc, "", "  ")
	os.WriteFile(docFile, data, 0644)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"write", docFile})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Written: written-doc") {
		t.Errorf("expected success message, got: %s", output)
	}

	// Verify doc exists on disk.
	docPath := filepath.Join(dir, ".leo", "kb", "docs", "written-doc.json")
	if _, err := os.Stat(docPath); err != nil {
		t.Error("doc file not created")
	}
}

func TestWriteCmd_InvalidDoc(t *testing.T) {
	dir := setupTestKB(t)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	docFile := filepath.Join(dir, "bad-doc.json")
	os.WriteFile(docFile, []byte(`{"id": "INVALID"}`), 0644)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"write", docFile})

	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error for invalid doc")
	}
}

func TestQueryCmd_FiltersByType(t *testing.T) {
	dir := setupTestKB(t)

	docA := sampleDoc("query-a")
	docA.Type = "fact"
	writeTestDoc(t, dir, docA)

	docB := sampleDoc("query-b")
	docB.Type = "rule"
	docB.Content = map[string]any{
		"rule": "r", "why": "w", "how_to_apply": []any{"h"}, "responsibility": "r",
	}
	writeTestDoc(t, dir, docB)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"query", "--type", "fact"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("query failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "query-a") {
		t.Error("expected query-a in results")
	}
	if strings.Contains(output, "query-b") {
		t.Error("did not expect query-b in results")
	}
}

func TestQueryCmd_NoResults(t *testing.T) {
	dir := setupTestKB(t)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"query", "--type", "metric"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("query failed: %v", err)
	}

	if !strings.Contains(buf.String(), "No documents found") {
		t.Errorf("expected 'No documents found', got: %s", buf.String())
	}
}

func TestDeleteCmd_RequiresForce(t *testing.T) {
	dir := setupTestKB(t)
	writeTestDoc(t, dir, sampleDoc("keep-me"))

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"delete", "keep-me"})

	rootCmd.Execute()

	// Doc should still exist (no --force).
	docPath := filepath.Join(dir, ".leo", "kb", "docs", "keep-me.json")
	if _, err := os.Stat(docPath); err != nil {
		t.Error("doc should not be deleted without --force")
	}
}

func TestDeleteCmd_WithForce(t *testing.T) {
	dir := setupTestKB(t)
	writeTestDoc(t, dir, sampleDoc("delete-me"))

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"delete", "delete-me", "--force"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("delete --force failed: %v", err)
	}

	docPath := filepath.Join(dir, ".leo", "kb", "docs", "delete-me.json")
	if _, err := os.Stat(docPath); err == nil {
		t.Error("doc should be deleted with --force")
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
