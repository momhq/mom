package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// coreDoc returns a minimal valid KB doc as a JSON byte slice.
func coreDoc(id string, scope string, updated time.Time) []byte {
	doc := map[string]any{
		"id":         id,
		"type":       "rule",
		"lifecycle":  "permanent",
		"scope":      scope,
		"tags":       []string{"test"},
		"created":    "2026-04-10T00:00:00Z",
		"created_by": "test",
		"updated":    updated.UTC().Format(time.RFC3339),
		"updated_by": "test",
		"content": map[string]any{
			"summary": "Test rule " + id,
			"body":    "This is test rule " + id + ".",
		},
	}
	data, _ := json.MarshalIndent(doc, "", "  ")
	return append(data, '\n')
}

// setupFakeCore creates a temp directory with the leo-core structure:
//
//	tmpCore/.claude/kb/docs/
//	tmpCore/.claude/kb/schema.json
//
// It returns the path to the tmpCore directory.
func setupFakeCore(t *testing.T) string {
	t.Helper()
	core := t.TempDir()

	docsDir := filepath.Join(core, ".claude", "kb", "docs")
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		t.Fatalf("setupFakeCore: creating docs dir: %v", err)
	}

	schema := []byte(`{"version":"1","description":"Leo KB schema"}`)
	if err := os.WriteFile(filepath.Join(core, ".claude", "kb", "schema.json"), schema, 0644); err != nil {
		t.Fatalf("setupFakeCore: writing schema: %v", err)
	}

	return core
}

// addCoreDoc writes a doc JSON file into {core}/.claude/kb/docs/.
func addCoreDoc(t *testing.T, core, id, scope string, updated time.Time) {
	t.Helper()
	path := filepath.Join(core, ".claude", "kb", "docs", id+".json")
	if err := os.WriteFile(path, coreDoc(id, scope, updated), 0644); err != nil {
		t.Fatalf("addCoreDoc: %v", err)
	}
}

// setupFakeProject creates a temp directory with the project .leo/ structure.
func setupFakeProject(t *testing.T) string {
	t.Helper()
	proj := t.TempDir()
	leoDir := filepath.Join(proj, ".leo")
	dirs := []string{
		leoDir,
		filepath.Join(leoDir, "kb", "docs"),
		filepath.Join(leoDir, "cache"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("setupFakeProject: creating dir %s: %v", d, err)
		}
	}

	// Minimal config.yaml.
	cfg := []byte("version: \"1\"\nruntime: claude\n")
	if err := os.WriteFile(filepath.Join(leoDir, "config.yaml"), cfg, 0644); err != nil {
		t.Fatalf("setupFakeProject: writing config: %v", err)
	}

	// Empty index.json.
	emptyIdx := []byte(`{"version":"1","last_rebuilt":"","by_tag":{},"by_type":{},"by_scope":{},"by_lifecycle":{}}` + "\n")
	if err := os.WriteFile(filepath.Join(leoDir, "kb", "index.json"), emptyIdx, 0644); err != nil {
		t.Fatalf("setupFakeProject: writing index: %v", err)
	}

	// Minimal schema.json (different from core default).
	schema := []byte(`{"version":"0","description":"old schema"}`)
	if err := os.WriteFile(filepath.Join(leoDir, "kb", "schema.json"), schema, 0644); err != nil {
		t.Fatalf("setupFakeProject: writing schema: %v", err)
	}

	return proj
}

// runUpdateCmd sets up rootCmd to run "update" with given args in the given
// working directory (changes os cwd temporarily).
// It resets updateCmd flags before each run to prevent cobra state leaking
// between tests.
func runUpdateCmd(t *testing.T, projDir string, args []string) (string, error) {
	t.Helper()

	// Reset updateCmd flags to defaults to prevent cross-test contamination.
	updateCmd.Flags().Set("source", "")
	updateCmd.Flags().Set("dry-run", "false")
	updateCmd.Flags().Set("yes", "false")

	origDir, _ := os.Getwd()
	os.Chdir(filepath.Join(projDir)) // project root, not .leo/
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(append([]string{"update"}, args...))

	err := rootCmd.Execute()
	return buf.String(), err
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestUpdateCmd_NoCoreSource(t *testing.T) {
	proj := setupFakeProject(t)

	_, err := runUpdateCmd(t, proj, []string{})
	if err == nil {
		t.Fatal("expected error when no core source is configured")
	}
	if !strings.Contains(err.Error(), "no core source configured") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUpdateCmd_InvalidSource(t *testing.T) {
	proj := setupFakeProject(t)

	_, err := runUpdateCmd(t, proj, []string{"--source", "/nonexistent/path"})
	if err == nil {
		t.Fatal("expected error for invalid source path")
	}
	if !strings.Contains(err.Error(), ".claude/kb/docs") {
		t.Errorf("expected error about missing .claude/kb/docs, got: %v", err)
	}
}

func TestUpdateCmd_DryRun(t *testing.T) {
	proj := setupFakeProject(t)
	core := setupFakeCore(t)

	t1 := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	addCoreDoc(t, core, "rule-one", "core", t1)
	addCoreDoc(t, core, "rule-two", "core", t1)

	out, err := runUpdateCmd(t, proj, []string{"--source", core, "--dry-run"})
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}

	// Should show plan but not write files.
	if !strings.Contains(out, "rule-one") {
		t.Errorf("expected 'rule-one' in dry-run output, got:\n%s", out)
	}
	if !strings.Contains(out, "rule-two") {
		t.Errorf("expected 'rule-two' in dry-run output, got:\n%s", out)
	}

	// No files should have been copied.
	entries, _ := os.ReadDir(filepath.Join(proj, ".leo", "kb", "docs"))
	if len(entries) > 0 {
		t.Errorf("dry-run should not write files, found %d file(s)", len(entries))
	}
}

func TestUpdateCmd_AddsNewDocs(t *testing.T) {
	proj := setupFakeProject(t)
	core := setupFakeCore(t)

	t1 := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	addCoreDoc(t, core, "evidence-over-claim", "core", t1)
	addCoreDoc(t, core, "anti-hallucination", "core", t1)

	_, err := runUpdateCmd(t, proj, []string{"--source", core, "--yes"})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	docsDir := filepath.Join(proj, ".leo", "kb", "docs")
	for _, id := range []string{"evidence-over-claim", "anti-hallucination"} {
		p := filepath.Join(docsDir, id+".json")
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected doc %s to be created, but it's missing", id)
		}
	}
}

func TestUpdateCmd_UpdatesNewerDocs(t *testing.T) {
	proj := setupFakeProject(t)
	core := setupFakeCore(t)

	t1 := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)

	// Core has t2, project has t1 (older).
	addCoreDoc(t, core, "my-rule", "core", t2)

	// Write project doc with t1.
	projDocPath := filepath.Join(proj, ".leo", "kb", "docs", "my-rule.json")
	if err := os.WriteFile(projDocPath, coreDoc("my-rule", "core", t1), 0644); err != nil {
		t.Fatalf("writing project doc: %v", err)
	}

	_, err := runUpdateCmd(t, proj, []string{"--source", core, "--yes"})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	// The project file should now have the newer content (t2 timestamp).
	data, err := os.ReadFile(projDocPath)
	if err != nil {
		t.Fatalf("reading updated doc: %v", err)
	}
	if !strings.Contains(string(data), t2.UTC().Format(time.RFC3339)) {
		t.Errorf("expected doc to be updated to t2 timestamp, got:\n%s", string(data))
	}
}

func TestUpdateCmd_SkipsUnchangedDocs(t *testing.T) {
	proj := setupFakeProject(t)
	core := setupFakeCore(t)

	ts := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	addCoreDoc(t, core, "stable-rule", "core", ts)

	// Project already has the same doc with the same timestamp.
	projDocPath := filepath.Join(proj, ".leo", "kb", "docs", "stable-rule.json")
	origContent := coreDoc("stable-rule", "core", ts)
	if err := os.WriteFile(projDocPath, origContent, 0644); err != nil {
		t.Fatalf("writing project doc: %v", err)
	}

	// Record modification time before update.
	info, _ := os.Stat(projDocPath)
	modBefore := info.ModTime()

	_, err := runUpdateCmd(t, proj, []string{"--source", core, "--yes"})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	// File should not have been touched (same mod time).
	info2, _ := os.Stat(projDocPath)
	if !info2.ModTime().Equal(modBefore) {
		t.Errorf("unchanged doc should not have been written (mod time changed)")
	}
}

func TestUpdateCmd_PreservesProjectDocs(t *testing.T) {
	proj := setupFakeProject(t)
	core := setupFakeCore(t)

	// Core has one core-scoped doc.
	t1 := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	addCoreDoc(t, core, "core-rule", "core", t1)

	// Project has its own project-scoped doc.
	projDocPath := filepath.Join(proj, ".leo", "kb", "docs", "project-decision.json")
	projDocContent := coreDoc("project-decision", "project", t1)
	if err := os.WriteFile(projDocPath, projDocContent, 0644); err != nil {
		t.Fatalf("writing project doc: %v", err)
	}

	_, err := runUpdateCmd(t, proj, []string{"--source", core, "--yes"})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	// Project-scoped doc should still be there, untouched.
	if _, err := os.Stat(projDocPath); err != nil {
		t.Errorf("project-scoped doc was deleted or moved: %v", err)
	}
}

func TestUpdateCmd_SyncsSchema(t *testing.T) {
	proj := setupFakeProject(t)
	core := setupFakeCore(t)

	// Core schema is already different from project schema (set in setupFakeProject).
	// setupFakeCore writes: {"version":"1","description":"Leo KB schema"}
	// setupFakeProject writes: {"version":"0","description":"old schema"}

	_, err := runUpdateCmd(t, proj, []string{"--source", core, "--yes"})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	// Project schema should now match core schema.
	projSchema, err := os.ReadFile(filepath.Join(proj, ".leo", "kb", "schema.json"))
	if err != nil {
		t.Fatalf("reading project schema: %v", err)
	}
	coreSchema, err := os.ReadFile(filepath.Join(core, ".claude", "kb", "schema.json"))
	if err != nil {
		t.Fatalf("reading core schema: %v", err)
	}
	if !bytes.Equal(projSchema, coreSchema) {
		t.Errorf("project schema not updated to match core:\nproject: %s\ncore:    %s", projSchema, coreSchema)
	}
}

func TestUpdateCmd_SkipsNonCoreScopeDocs(t *testing.T) {
	proj := setupFakeProject(t)
	core := setupFakeCore(t)

	t1 := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	// Add a project-scoped doc to the core (should be ignored).
	addCoreDoc(t, core, "project-specific", "project", t1)
	// Add one legitimate core-scoped doc.
	addCoreDoc(t, core, "real-core-rule", "core", t1)

	_, err := runUpdateCmd(t, proj, []string{"--source", core, "--yes"})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	docsDir := filepath.Join(proj, ".leo", "kb", "docs")

	// project-specific should NOT be copied.
	if _, err := os.Stat(filepath.Join(docsDir, "project-specific.json")); err == nil {
		t.Errorf("project-scoped doc from core should not have been copied")
	}

	// real-core-rule should be copied.
	if _, err := os.Stat(filepath.Join(docsDir, "real-core-rule.json")); err != nil {
		t.Errorf("core-scoped doc should have been copied: %v", err)
	}
}
