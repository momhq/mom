package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Export tests ---

func TestExportCmd_CreatesDefaultDirStructure(t *testing.T) {
	dir := setupTestMemory(t)
	writeTestDoc(t, dir, sampleDoc("export-doc-1"))
	writeTestDoc(t, dir, sampleDoc("export-doc-2"))

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	exportCmd.Flags().Set("output", "") //nolint:errcheck

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"export"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("export failed: %v", err)
	}

	// Default output: ./mom-export/
	exportDir := filepath.Join(dir, "mom-export")
	if _, err := os.Stat(exportDir); err != nil {
		t.Fatalf("mom-export/ directory not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(exportDir, "docs")); err != nil {
		t.Error("mom-export/docs/ not created")
	}
	if _, err := os.Stat(filepath.Join(exportDir, "index.json")); err != nil {
		t.Error("mom-export/index.json not created")
	}
}

func TestExportCmd_CopiesAllDocs(t *testing.T) {
	dir := setupTestMemory(t)
	writeTestDoc(t, dir, sampleDoc("export-alpha"))
	writeTestDoc(t, dir, sampleDoc("export-beta"))
	writeTestDoc(t, dir, sampleDoc("export-gamma"))

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	exportCmd.Flags().Set("output", "") //nolint:errcheck

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"export"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("export failed: %v", err)
	}

	exportDocsDir := filepath.Join(dir, "mom-export", "docs")
	for _, id := range []string{"export-alpha", "export-beta", "export-gamma"} {
		docPath := filepath.Join(exportDocsDir, id+".json")
		if _, err := os.Stat(docPath); err != nil {
			t.Errorf("exported doc %s.json not found", id)
		}
	}

	if !strings.Contains(buf.String(), "3") {
		t.Errorf("expected output to mention 3 docs, got: %s", buf.String())
	}
}

func TestExportCmd_CustomOutputPath(t *testing.T) {
	dir := setupTestMemory(t)
	writeTestDoc(t, dir, sampleDoc("export-custom"))

	customOut := filepath.Join(dir, "my-custom-export")

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"export", "--output", customOut})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("export --output failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(customOut, "docs")); err != nil {
		t.Error("custom output docs/ dir not created")
	}
	if _, err := os.Stat(filepath.Join(customOut, "index.json")); err != nil {
		t.Error("custom output index.json not created")
	}
	if _, err := os.Stat(filepath.Join(customOut, "docs", "export-custom.json")); err != nil {
		t.Error("custom output doc not found")
	}
}

func TestExportCmd_CopiesSchema(t *testing.T) {
	dir := setupTestMemory(t)

	// Write a schema.json into the .mom/ directory.
	schemaPath := filepath.Join(dir, ".mom", "schema.json")
	os.WriteFile(schemaPath, []byte(`{"version":"1"}`), 0644)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Reset the --output flag to empty before this test (guard against shared state).
	exportCmd.Flags().Set("output", "") //nolint:errcheck

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"export"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("export failed: %v", err)
	}

	exportedSchema := filepath.Join(dir, "mom-export", "schema.json")
	if _, err := os.Stat(exportedSchema); err != nil {
		t.Error("schema.json not exported")
	}
}

// --- Import tests ---

func TestImportCmd_MergeMode_AddsNewDocs(t *testing.T) {
	// Source memory: has doc-a, doc-b
	srcDir := setupTestMemory(t)
	writeTestDoc(t, srcDir, sampleDoc("import-new-a"))
	writeTestDoc(t, srcDir, sampleDoc("import-new-b"))

	// Export the source memory.
	exportDir := filepath.Join(srcDir, "export-src")
	os.MkdirAll(filepath.Join(exportDir, "docs"), 0755)
	copyDir(t,
		filepath.Join(srcDir, ".mom", "memory"),
		filepath.Join(exportDir, "docs"),
	)
	copyFile(t,
		filepath.Join(srcDir, ".mom", "index.json"),
		filepath.Join(exportDir, "index.json"),
	)

	// Destination memory: empty.
	destDir := setupTestMemory(t)

	origDir, _ := os.Getwd()
	os.Chdir(destDir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"import", exportDir})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("import failed: %v", err)
	}

	// Both docs should now exist in the destination.
	for _, id := range []string{"import-new-a", "import-new-b"} {
		docPath := filepath.Join(destDir, ".mom", "memory", id+".json")
		if _, err := os.Stat(docPath); err != nil {
			t.Errorf("imported doc %s not found in dest memory", id)
		}
	}

	if !strings.Contains(buf.String(), "2") {
		t.Errorf("expected output to mention 2 imported docs, got: %s", buf.String())
	}
}

func TestImportCmd_MergeMode_SkipsExistingDocs(t *testing.T) {
	// Source memory: has existing-doc with content "original".
	srcDir := setupTestMemory(t)
	doc := sampleDoc("existing-doc")
	doc.Content = map[string]any{"fact": "original"}
	writeTestDoc(t, srcDir, doc)

	// Export it.
	exportDir := filepath.Join(srcDir, "export-src")
	os.MkdirAll(filepath.Join(exportDir, "docs"), 0755)
	copyDir(t,
		filepath.Join(srcDir, ".mom", "memory"),
		filepath.Join(exportDir, "docs"),
	)
	copyFile(t,
		filepath.Join(srcDir, ".mom", "index.json"),
		filepath.Join(exportDir, "index.json"),
	)

	// Destination memory: already has existing-doc with content "local".
	destDir := setupTestMemory(t)
	localDoc := sampleDoc("existing-doc")
	localDoc.Content = map[string]any{"fact": "local"}
	writeTestDoc(t, destDir, localDoc)

	origDir, _ := os.Getwd()
	os.Chdir(destDir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"import", "--merge", exportDir})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("import --merge failed: %v", err)
	}

	// The destination doc should still have "local" content.
	data, err := os.ReadFile(filepath.Join(destDir, ".mom", "memory", "existing-doc.json"))
	if err != nil {
		t.Fatalf("reading destination doc: %v", err)
	}
	var result map[string]any
	json.Unmarshal(data, &result)
	content, _ := result["content"].(map[string]any)
	if content["fact"] != "local" {
		t.Errorf("merge should preserve local doc, got content: %v", content)
	}

	if !strings.Contains(buf.String(), "skipped") || !strings.Contains(buf.String(), "1") {
		t.Errorf("expected output to mention 1 skipped doc, got: %s", buf.String())
	}
}

func TestImportCmd_ReplaceMode_BacksUpFirst(t *testing.T) {
	// Source memory: has source-doc.
	srcDir := setupTestMemory(t)
	writeTestDoc(t, srcDir, sampleDoc("source-doc"))

	// Export it.
	exportDir := filepath.Join(srcDir, "export-src")
	os.MkdirAll(filepath.Join(exportDir, "docs"), 0755)
	copyDir(t,
		filepath.Join(srcDir, ".mom", "memory"),
		filepath.Join(exportDir, "docs"),
	)
	copyFile(t,
		filepath.Join(srcDir, ".mom", "index.json"),
		filepath.Join(exportDir, "index.json"),
	)

	// Destination memory: has existing-doc.
	destDir := setupTestMemory(t)
	writeTestDoc(t, destDir, sampleDoc("existing-before-replace"))

	origDir, _ := os.Getwd()
	os.Chdir(destDir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"import", "--replace", exportDir})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("import --replace failed: %v", err)
	}

	// A backup directory should exist at the flat level.
	leoDir := filepath.Join(destDir, ".mom")
	entries, err := os.ReadDir(leoDir)
	if err != nil {
		t.Fatalf("reading .mom/: %v", err)
	}

	foundBackup := false
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "backup-") {
			foundBackup = true
			// Backup should contain existing-before-replace.
			backupDoc := filepath.Join(leoDir, e.Name(), "docs", "existing-before-replace.json")
			if _, err := os.Stat(backupDoc); err != nil {
				t.Error("backup does not contain original doc")
			}
		}
	}
	if !foundBackup {
		t.Error("no backup- directory found after --replace import")
	}
}

func TestImportCmd_ReplaceMode_ReplacesAllDocs(t *testing.T) {
	// Source memory: has source-doc.
	srcDir := setupTestMemory(t)
	writeTestDoc(t, srcDir, sampleDoc("source-doc-replace"))

	// Export it.
	exportDir := filepath.Join(srcDir, "export-src")
	os.MkdirAll(filepath.Join(exportDir, "docs"), 0755)
	copyDir(t,
		filepath.Join(srcDir, ".mom", "memory"),
		filepath.Join(exportDir, "docs"),
	)
	copyFile(t,
		filepath.Join(srcDir, ".mom", "index.json"),
		filepath.Join(exportDir, "index.json"),
	)

	// Destination memory: has old-doc that should be gone after replace.
	destDir := setupTestMemory(t)
	writeTestDoc(t, destDir, sampleDoc("old-doc-to-be-gone"))

	origDir, _ := os.Getwd()
	os.Chdir(destDir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"import", "--replace", exportDir})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("import --replace failed: %v", err)
	}

	// source-doc-replace should exist.
	if _, err := os.Stat(filepath.Join(destDir, ".mom", "memory", "source-doc-replace.json")); err != nil {
		t.Error("source doc not found after replace import")
	}

	// old-doc-to-be-gone should NOT exist.
	if _, err := os.Stat(filepath.Join(destDir, ".mom", "memory", "old-doc-to-be-gone.json")); err == nil {
		t.Error("old doc should be gone after replace import, but still exists")
	}
}

func TestImportCmd_ValidatesSchema(t *testing.T) {
	// Create an import source with an invalid doc.
	srcDir := t.TempDir()
	importDir := filepath.Join(srcDir, "import-src")
	os.MkdirAll(filepath.Join(importDir, "docs"), 0755)

	// Valid index.
	idx := map[string]any{
		"version": "1", "last_rebuilt": "", "by_tag": map[string]any{},
		"by_type": map[string]any{}, "by_scope": map[string]any{},
		"by_lifecycle": map[string]any{},
	}
	idxData, _ := json.MarshalIndent(idx, "", "  ")
	os.WriteFile(filepath.Join(importDir, "index.json"), idxData, 0644)

	// Invalid doc (missing required fields).
	invalidDoc := map[string]any{"id": "INVALID_ID", "type": "fact"}
	invalidData, _ := json.MarshalIndent(invalidDoc, "", "  ")
	os.WriteFile(filepath.Join(importDir, "docs", "INVALID_ID.json"), invalidData, 0644)

	destDir := setupTestMemory(t)

	origDir, _ := os.Getwd()
	os.Chdir(destDir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"import", importDir})

	// Should not hard fail — errors are counted and reported.
	rootCmd.Execute()

	output := buf.String()
	if !strings.Contains(output, "error") && !strings.Contains(output, "invalid") {
		t.Errorf("expected error/invalid mention in output, got: %s", output)
	}
}

func TestImportCmd_RebuildsIndexAfterImport(t *testing.T) {
	srcDir := setupTestMemory(t)
	writeTestDoc(t, srcDir, sampleDoc("index-rebuild-doc"))

	exportDir := filepath.Join(srcDir, "export-src")
	os.MkdirAll(filepath.Join(exportDir, "docs"), 0755)
	copyDir(t,
		filepath.Join(srcDir, ".mom", "memory"),
		filepath.Join(exportDir, "docs"),
	)
	copyFile(t,
		filepath.Join(srcDir, ".mom", "index.json"),
		filepath.Join(exportDir, "index.json"),
	)

	destDir := setupTestMemory(t)

	// Corrupt the destination index.
	indexPath := filepath.Join(destDir, ".mom", "index.json")
	os.WriteFile(indexPath, []byte(`{}`), 0644)

	origDir, _ := os.Getwd()
	os.Chdir(destDir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"import", exportDir})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("import failed: %v", err)
	}

	// Index should now reference index-rebuild-doc.
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("reading index: %v", err)
	}
	if !strings.Contains(string(indexData), "index-rebuild-doc") {
		t.Error("index not rebuilt: does not reference imported doc")
	}
}

func TestExportImport_RoundTrip(t *testing.T) {
	// Set up original memory with two docs.
	origKBDir := setupTestMemory(t)
	writeTestDoc(t, origKBDir, sampleDoc("roundtrip-alpha"))
	writeTestDoc(t, origKBDir, sampleDoc("roundtrip-beta"))

	// Export.
	exportDir := filepath.Join(origKBDir, "roundtrip-export")

	origDir, _ := os.Getwd()
	os.Chdir(origKBDir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"export", "--output", exportDir})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("export failed: %v", err)
	}

	// New empty memory.
	newKBDir := setupTestMemory(t)
	os.Chdir(newKBDir)

	buf.Reset()
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"import", exportDir})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("import failed: %v", err)
	}

	// Both docs should be present.
	for _, id := range []string{"roundtrip-alpha", "roundtrip-beta"} {
		docPath := filepath.Join(newKBDir, ".mom", "memory", id+".json")
		if _, err := os.Stat(docPath); err != nil {
			t.Errorf("round-trip: doc %s not found in imported memory", id)
		}
	}
}

// --- Test helpers ---

// copyFile copies a single file from src to dst.
func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("copyFile read %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		t.Fatalf("copyFile write %s: %v", dst, err)
	}
}

// copyDir copies all .json files from srcDir to dstDir.
func copyDir(t *testing.T, srcDir, dstDir string) {
	t.Helper()
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatalf("copyDir read %s: %v", srcDir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		copyFile(t, filepath.Join(srcDir, e.Name()), filepath.Join(dstDir, e.Name()))
	}
}
