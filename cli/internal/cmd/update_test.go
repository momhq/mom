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
		"type":       "constraint",
		"lifecycle":  "permanent",
		"scope":      scope,
		"tags":       []string{"test"},
		"created":    "2026-04-10T00:00:00Z",
		"created_by": "test",
		"updated":    updated.UTC().Format(time.RFC3339),
		"updated_by": "test",
		"content": map[string]any{
			"constraint":   "Test constraint " + id,
			"why":          "Testing",
			"how_to_apply": []string{"Apply " + id},
		},
	}
	data, _ := json.MarshalIndent(doc, "", "  ")
	return append(data, '\n')
}

// setupFakeCore creates a temp directory with the leo-core structure (new flat layout):
//
//	tmpCore/.mom/memory/
//	tmpCore/.mom/schema.json
//
// It returns the path to the tmpCore directory.
func setupFakeCore(t *testing.T) string {
	t.Helper()
	core := t.TempDir()

	docsDir := filepath.Join(core, ".mom", "memory")
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		t.Fatalf("setupFakeCore: creating memory dir: %v", err)
	}

	schema := []byte(`{"version":"1","description":"Leo KB schema"}`)
	if err := os.WriteFile(filepath.Join(core, ".mom", "schema.json"), schema, 0644); err != nil {
		t.Fatalf("setupFakeCore: writing schema: %v", err)
	}

	profilesDir := filepath.Join(core, ".mom", "profiles")
	if err := os.MkdirAll(profilesDir, 0755); err != nil {
		t.Fatalf("setupFakeCore: creating profiles dir: %v", err)
	}

	// Identity.json.
	identity := []byte(`{"name":"Leo","version":"1"}`)
	if err := os.WriteFile(filepath.Join(core, ".mom", "identity.json"), identity, 0644); err != nil {
		t.Fatalf("setupFakeCore: writing identity.json: %v", err)
	}

	return core
}

// addCoreDoc writes a doc JSON file into {core}/.mom/memory/.
func addCoreDoc(t *testing.T, core, id, scope string, updated time.Time) {
	t.Helper()
	path := filepath.Join(core, ".mom", "memory", id+".json")
	if err := os.WriteFile(path, coreDoc(id, scope, updated), 0644); err != nil {
		t.Fatalf("addCoreDoc: %v", err)
	}
}

// setupFakeProject creates a temp directory with the project .mom/ structure (new flat layout).
func setupFakeProject(t *testing.T) string {
	t.Helper()
	proj := t.TempDir()
	leoDir := filepath.Join(proj, ".mom")
	dirs := []string{
		leoDir,
		filepath.Join(leoDir, "memory"),
		filepath.Join(leoDir, "profiles"),
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
	if err := os.WriteFile(filepath.Join(leoDir, "index.json"), emptyIdx, 0644); err != nil {
		t.Fatalf("setupFakeProject: writing index: %v", err)
	}

	// Minimal schema.json (different from core default).
	schema := []byte(`{"version":"0","description":"old schema"}`)
	if err := os.WriteFile(filepath.Join(leoDir, "schema.json"), schema, 0644); err != nil {
		t.Fatalf("setupFakeProject: writing schema: %v", err)
	}

	return proj
}

// runUpdateCmdWithInput sets up rootCmd to run "update" with given args in the
// given working directory (changes os cwd temporarily). It also sets stdin to
// the provided input string for interactive prompts.
// It resets updateCmd flags before each run to prevent cobra state leaking
// between tests.
func runUpdateCmdWithInput(t *testing.T, projDir string, args []string, input string) (string, error) {
	t.Helper()

	// Reset updateCmd flags to defaults to prevent cross-test contamination.
	updateCmd.Flags().Set("source", "")
	updateCmd.Flags().Set("dry-run", "false")
	updateCmd.Flags().Set("yes", "false")

	origDir, _ := os.Getwd()
	os.Chdir(filepath.Join(projDir)) // project root, not .mom/
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetIn(strings.NewReader(input))
	rootCmd.SetArgs(append([]string{"update"}, args...))

	err := rootCmd.Execute()
	return buf.String(), err
}

// runUpdateCmd sets up rootCmd to run "update" with given args in the given
// working directory (changes os cwd temporarily).
// It resets updateCmd flags before each run to prevent cobra state leaking
// between tests.
func runUpdateCmd(t *testing.T, projDir string, args []string) (string, error) {
	t.Helper()
	return runUpdateCmdWithInput(t, projDir, args, "")
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
	if !strings.Contains(err.Error(), ".mom/memory") && !strings.Contains(err.Error(), ".mom/kb/docs") {
		t.Errorf("expected error about missing memory dir, got: %v", err)
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
	entries, _ := os.ReadDir(filepath.Join(proj, ".mom", "memory"))
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

	docsDir := filepath.Join(proj, ".mom", "memory")
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
	projDocPath := filepath.Join(proj, ".mom", "memory", "my-rule.json")
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
	projDocPath := filepath.Join(proj, ".mom", "memory", "stable-rule.json")
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
	projDocPath := filepath.Join(proj, ".mom", "memory", "project-decision.json")
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
	projSchema, err := os.ReadFile(filepath.Join(proj, ".mom", "schema.json"))
	if err != nil {
		t.Fatalf("reading project schema: %v", err)
	}
	coreSchema, err := os.ReadFile(filepath.Join(core, ".mom", "schema.json"))
	if err != nil {
		t.Fatalf("reading core schema: %v", err)
	}
	if !bytes.Equal(projSchema, coreSchema) {
		t.Errorf("project schema not updated to match core:\nproject: %s\ncore:    %s", projSchema, coreSchema)
	}
}

// addCoreProfile writes a profile YAML file into {core}/.mom/profiles/.
func addCoreProfile(t *testing.T, core, name, content string) {
	t.Helper()
	dir := filepath.Join(core, ".mom", "profiles")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("addCoreProfile: creating profiles dir: %v", err)
	}
	path := filepath.Join(dir, name+".yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("addCoreProfile: %v", err)
	}
}

// addProjectProfile writes a profile YAML file into {proj}/.mom/profiles/.
func addProjectProfile(t *testing.T, proj, name, content string) {
	t.Helper()
	dir := filepath.Join(proj, ".mom", "profiles")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("addProjectProfile: creating profiles dir: %v", err)
	}
	path := filepath.Join(dir, name+".yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("addProjectProfile: %v", err)
	}
}

func TestUpdateCmd_AddsNewProfiles(t *testing.T) {
	proj := setupFakeProject(t)
	core := setupFakeCore(t)

	addCoreProfile(t, core, "go-specialist", "name: go-specialist\nmodel: sonnet\n")
	addCoreProfile(t, core, "security-reviewer", "name: security-reviewer\nmodel: opus\n")

	out, err := runUpdateCmd(t, proj, []string{"--source", core, "--yes"})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	// Both profiles should have been copied.
	profilesDir := filepath.Join(proj, ".mom", "profiles")
	for _, name := range []string{"go-specialist", "security-reviewer"} {
		p := filepath.Join(profilesDir, name+".yaml")
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected profile %s to be created, but it's missing", name)
		}
	}

	// Output should mention profiles.
	if !strings.Contains(out, "go-specialist") {
		t.Errorf("expected 'go-specialist' in output, got:\n%s", out)
	}
}

func TestUpdateCmd_KeepsConflictingProfilesWithYes(t *testing.T) {
	proj := setupFakeProject(t)
	core := setupFakeCore(t)

	oldContent := "name: go-specialist\nmodel: sonnet\n"
	newContent := "name: go-specialist\nmodel: opus\nskills: [refactor]\n"

	// Project has old version, core has new version.
	addProjectProfile(t, proj, "go-specialist", oldContent)
	addCoreProfile(t, core, "go-specialist", newContent)

	_, err := runUpdateCmd(t, proj, []string{"--source", core, "--yes"})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	// With --yes, safe default is to KEEP local (don't replace).
	data, err := os.ReadFile(filepath.Join(proj, ".mom", "profiles", "go-specialist.yaml"))
	if err != nil {
		t.Fatalf("reading profile: %v", err)
	}
	if string(data) != oldContent {
		t.Errorf("expected local profile to be kept with --yes, got:\n%s", string(data))
	}
}

func TestUpdateCmd_ReplacesConflictProfileInteractively(t *testing.T) {
	proj := setupFakeProject(t)
	core := setupFakeCore(t)

	oldContent := "name: go-specialist\nmodel: sonnet\n"
	newContent := "name: go-specialist\nmodel: opus\nskills: [refactor]\n"

	addProjectProfile(t, proj, "go-specialist", oldContent)
	addCoreProfile(t, core, "go-specialist", newContent)

	// First "y" for "Apply changes?", second "y" for the profile conflict.
	out, err := runUpdateCmdWithInput(t, proj, []string{"--source", core}, "y\ny\n")
	if err != nil {
		t.Fatalf("update failed: %v\noutput:\n%s", err, out)
	}

	// Profile should have been replaced with core version.
	data, err := os.ReadFile(filepath.Join(proj, ".mom", "profiles", "go-specialist.yaml"))
	if err != nil {
		t.Fatalf("reading profile: %v", err)
	}
	if string(data) != newContent {
		t.Errorf("expected profile to be replaced with core version, got:\n%s", string(data))
	}
}

func TestUpdateCmd_KeepsConflictProfileInteractively(t *testing.T) {
	proj := setupFakeProject(t)
	core := setupFakeCore(t)

	oldContent := "name: go-specialist\nmodel: sonnet\n"
	newContent := "name: go-specialist\nmodel: opus\nskills: [refactor]\n"

	addProjectProfile(t, proj, "go-specialist", oldContent)
	addCoreProfile(t, core, "go-specialist", newContent)

	// First "y" for "Apply changes?", second "n" for the profile conflict.
	out, err := runUpdateCmdWithInput(t, proj, []string{"--source", core}, "y\nn\n")
	if err != nil {
		t.Fatalf("update failed: %v\noutput:\n%s", err, out)
	}

	// Profile should still have the local content.
	data, err := os.ReadFile(filepath.Join(proj, ".mom", "profiles", "go-specialist.yaml"))
	if err != nil {
		t.Fatalf("reading profile: %v", err)
	}
	if string(data) != oldContent {
		t.Errorf("expected local profile to be kept, got:\n%s", string(data))
	}
}

func TestUpdateCmd_SkipsUnchangedProfiles(t *testing.T) {
	proj := setupFakeProject(t)
	core := setupFakeCore(t)

	content := "name: go-specialist\nmodel: sonnet\n"
	addProjectProfile(t, proj, "go-specialist", content)
	addCoreProfile(t, core, "go-specialist", content)

	// Record modification time before update.
	profPath := filepath.Join(proj, ".mom", "profiles", "go-specialist.yaml")
	info, _ := os.Stat(profPath)
	modBefore := info.ModTime()

	_, err := runUpdateCmd(t, proj, []string{"--source", core, "--yes"})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	// File should not have been touched.
	info2, _ := os.Stat(profPath)
	if !info2.ModTime().Equal(modBefore) {
		t.Errorf("unchanged profile should not have been written (mod time changed)")
	}
}

func TestUpdateCmd_DryRunShowsProfiles(t *testing.T) {
	proj := setupFakeProject(t)
	core := setupFakeCore(t)

	// New profile (no local version).
	addCoreProfile(t, core, "go-specialist", "name: go-specialist\nmodel: sonnet\n")

	// Conflicting profile (local differs from core).
	addProjectProfile(t, proj, "security-reviewer", "name: security-reviewer\nmodel: sonnet\n")
	addCoreProfile(t, core, "security-reviewer", "name: security-reviewer\nmodel: opus\n")

	out, err := runUpdateCmd(t, proj, []string{"--source", core, "--dry-run"})
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}

	// Should show profiles section with the profile name.
	if !strings.Contains(out, "Profiles:") {
		t.Errorf("expected 'Profiles:' section in dry-run output, got:\n%s", out)
	}
	if !strings.Contains(out, "go-specialist") {
		t.Errorf("expected 'go-specialist' in dry-run output, got:\n%s", out)
	}

	// Should show conflict marker for the differing profile.
	if !strings.Contains(out, "conflict") {
		t.Errorf("expected 'conflict' marker in dry-run output for differing profile, got:\n%s", out)
	}

	// No profile files should have been written.
	profilesDir := filepath.Join(proj, ".mom", "profiles")
	entries, _ := os.ReadDir(profilesDir)
	// Only the one we placed (security-reviewer) should be there, not go-specialist.
	for _, e := range entries {
		if e.Name() == "go-specialist.yaml" {
			t.Errorf("dry-run should not write new profiles")
		}
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

	docsDir := filepath.Join(proj, ".mom", "memory")

	// project-specific should NOT be copied.
	if _, err := os.Stat(filepath.Join(docsDir, "project-specific.json")); err == nil {
		t.Errorf("project-scoped doc from core should not have been copied")
	}

	// real-core-rule should be copied.
	if _, err := os.Stat(filepath.Join(docsDir, "real-core-rule.json")); err != nil {
		t.Errorf("core-scoped doc should have been copied: %v", err)
	}
}
