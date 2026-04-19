package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vmarinogg/leo-core/cli/internal/config"
)

// setupV060Project creates a .leo/ with v0.6.0-style config and minimal structure.
// resetUpgradeFlags resets cobra flag state between tests.
func resetUpgradeFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		upgradeCmd.Flags().Set("dry-run", "false")
	})
}

func setupV060Project(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	leoDir := filepath.Join(dir, ".leo")

	// Create directories (including profiles/ to simulate a legacy install).
	for _, d := range []string{
		leoDir,
		filepath.Join(leoDir, "profiles"),
		filepath.Join(leoDir, "kb", "docs"),
		filepath.Join(leoDir, "kb", "constraints"),
		filepath.Join(leoDir, "kb", "skills"),
		filepath.Join(leoDir, "cache"),
	} {
		os.MkdirAll(d, 0755)
	}

	// Write v0.6.0-style config (legacy format with "owner:" key).
	legacyConfig := `version: "1"
runtime: claude
owner:
  language: pt
  mode: caveman
  default_profile: cto
  autonomy: autonomous
kb:
  storage: json
  auto_propagate: true
  wrap_up: true
  stale_threshold: 30
`
	os.WriteFile(filepath.Join(leoDir, "config.yaml"), []byte(legacyConfig), 0644)

	// Write an old schema.json (different from current).
	os.WriteFile(filepath.Join(leoDir, "kb", "schema.json"), []byte(`{"old": true}`), 0644)

	// Write identity.json.
	os.WriteFile(filepath.Join(leoDir, "identity.json"), []byte(`{"old": true}`), 0644)

	// Write an old constraint.
	os.WriteFile(
		filepath.Join(leoDir, "kb", "constraints", "anti-hallucination.json"),
		[]byte(`{"id":"anti-hallucination","old":true}`),
		0644,
	)

	// Write retired constraint and skill files (simulating a pre-v0.8 install).
	os.WriteFile(
		filepath.Join(leoDir, "kb", "constraints", "delegation-mandatory.json"),
		[]byte(`{"id":"delegation-mandatory","type":"constraint"}`),
		0644,
	)
	os.WriteFile(
		filepath.Join(leoDir, "kb", "skills", "task-intake.json"),
		[]byte(`{"id":"task-intake","type":"skill"}`),
		0644,
	)

	// Write a profile file (will be removed by upgrade).
	os.WriteFile(
		filepath.Join(leoDir, "profiles", "general-manager.yaml"),
		[]byte("name: General Manager\ndescription: custom\n"),
		0644,
	)

	// Write a user doc that must survive upgrade.
	userDoc := map[string]interface{}{
		"id":         "my-decision",
		"type":       "decision",
		"lifecycle":  "learning",
		"scope":      "project",
		"tags":       []string{"architecture"},
		"created":    "2026-04-10T00:00:00Z",
		"created_by": "owner",
		"updated":    "2026-04-10T00:00:00Z",
		"updated_by": "owner",
		"content": map[string]interface{}{
			"decision":                "Use Go",
			"context":                 "Need a language",
			"why":                     "Performance",
			"alternatives_considered": []string{"Rust"},
			"impact":                  []string{"Fast builds"},
			"reversible":              true,
		},
	}
	docData, _ := json.MarshalIndent(userDoc, "", "  ")
	os.WriteFile(filepath.Join(leoDir, "kb", "docs", "my-decision.json"), docData, 0644)

	return dir
}

func TestUpgradeCmd_MigratesConfig(t *testing.T) {
	resetUpgradeFlags(t)
	dir := setupV060Project(t)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"upgrade"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("upgrade failed: %v\noutput:\n%s", err, buf.String())
	}

	// Config should be loadable and have runtimes map.
	cfg, err := config.Load(filepath.Join(dir, ".leo"))
	if err != nil {
		t.Fatalf("loading config after upgrade: %v", err)
	}
	if len(cfg.EnabledRuntimes()) == 0 {
		t.Error("expected at least one enabled runtime after migration")
	}

	// User settings should be preserved.
	if cfg.User.Language != "pt" {
		t.Errorf("expected language=pt preserved, got %q", cfg.User.Language)
	}
	if cfg.User.Autonomy != "autonomous" {
		t.Errorf("expected autonomy=autonomous preserved, got %q", cfg.User.Autonomy)
	}

	// communication.mode must be inferred (caveman → caveman).
	if cfg.Communication.Mode != "caveman" {
		t.Errorf("expected communication.mode=caveman, got %q", cfg.Communication.Mode)
	}
}

func TestUpgradeCmd_RemovesProfilesDir(t *testing.T) {
	resetUpgradeFlags(t)
	dir := setupV060Project(t)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"upgrade"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("upgrade failed: %v\noutput:\n%s", err, buf.String())
	}

	// profiles/ directory must be gone after upgrade.
	profilesDir := filepath.Join(dir, ".leo", "profiles")
	if _, err := os.Stat(profilesDir); err == nil {
		t.Error("profiles/ directory should have been removed by upgrade")
	}
}

func TestUpgradeCmd_RemovesRetiredConstraints(t *testing.T) {
	resetUpgradeFlags(t)
	dir := setupV060Project(t)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"upgrade"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("upgrade failed: %v\noutput:\n%s", err, buf.String())
	}

	leoDir := filepath.Join(dir, ".leo")

	// Retired constraints must be removed.
	for _, name := range []string{"delegation-mandatory", "think-before-execute", "know-what-you-dont-know", "peer-review-automatic"} {
		path := filepath.Join(leoDir, "kb", "constraints", name+".json")
		if _, err := os.Stat(path); err == nil {
			t.Errorf("retired constraint %s should have been removed", name)
		}
	}

	// Retired skill must be removed.
	taskIntakePath := filepath.Join(leoDir, "kb", "skills", "task-intake.json")
	if _, err := os.Stat(taskIntakePath); err == nil {
		t.Error("retired skill task-intake.json should have been removed")
	}

	// Active constraint must still exist.
	antiHalPath := filepath.Join(leoDir, "kb", "constraints", "anti-hallucination.json")
	if _, err := os.Stat(antiHalPath); err != nil {
		t.Error("active constraint anti-hallucination.json must survive upgrade")
	}
}

// TestUpgradeCmd_Idempotent verifies running upgrade twice is a no-op on the second run.
func TestUpgradeCmd_Idempotent(t *testing.T) {
	resetUpgradeFlags(t)
	dir := setupV060Project(t)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)

	// First upgrade.
	rootCmd.SetArgs([]string{"upgrade"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("first upgrade failed: %v\noutput:\n%s", err, buf.String())
	}

	// Second upgrade — should succeed without error.
	buf.Reset()
	rootCmd.SetArgs([]string{"upgrade"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("second upgrade (idempotent) failed: %v\noutput:\n%s", err, buf.String())
	}

	// profiles/ still gone.
	if _, err := os.Stat(filepath.Join(dir, ".leo", "profiles")); err == nil {
		t.Error("profiles/ should not reappear on second upgrade")
	}
}

func TestUpgradeCmd_UpdatesSchema(t *testing.T) {
	resetUpgradeFlags(t)
	dir := setupV060Project(t)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"upgrade"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}

	// Schema should be updated (not the old one).
	schema, err := os.ReadFile(filepath.Join(dir, ".leo", "kb", "schema.json"))
	if err != nil {
		t.Fatal("schema.json not found after upgrade")
	}
	if strings.Contains(string(schema), `"old"`) {
		t.Error("schema.json was not updated")
	}
	if !strings.Contains(string(schema), "session-log") {
		t.Error("schema.json missing session-log type")
	}
}

func TestUpgradeCmd_PreservesUserDocs(t *testing.T) {
	resetUpgradeFlags(t)
	dir := setupV060Project(t)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"upgrade"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}

	// User doc must still exist.
	docPath := filepath.Join(dir, ".leo", "kb", "docs", "my-decision.json")
	data, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatal("user doc my-decision.json was deleted during upgrade")
	}
	if !strings.Contains(string(data), "Use Go") {
		t.Error("user doc content was corrupted")
	}
}

func TestUpgradeCmd_CreatesLogsDir(t *testing.T) {
	resetUpgradeFlags(t)
	dir := setupV060Project(t)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"upgrade"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}

	// logs/ dir should exist now.
	logsDir := filepath.Join(dir, ".leo", "kb", "logs")
	info, err := os.Stat(logsDir)
	if err != nil {
		t.Fatal("logs dir not created during upgrade")
	}
	if !info.IsDir() {
		t.Error("logs is not a directory")
	}
}

func TestUpgradeCmd_DryRun(t *testing.T) {
	resetUpgradeFlags(t)
	dir := setupV060Project(t)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Read schema before.
	schemaBefore, _ := os.ReadFile(filepath.Join(dir, ".leo", "kb", "schema.json"))

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"upgrade", "--dry-run"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("upgrade --dry-run failed: %v", err)
	}

	// Schema should NOT have changed.
	schemaAfter, _ := os.ReadFile(filepath.Join(dir, ".leo", "kb", "schema.json"))
	if string(schemaBefore) != string(schemaAfter) {
		t.Error("dry-run modified schema.json")
	}

	// Output should mention dry run.
	if !strings.Contains(buf.String(), "Dry run") {
		t.Error("expected 'Dry run' in output")
	}

	// profiles/ should still exist (dry-run doesn't remove it).
	if _, err := os.Stat(filepath.Join(dir, ".leo", "profiles")); err != nil {
		t.Error("dry-run should not have removed profiles/")
	}
}

func TestUpgradeCmd_MigratesMetricDocs(t *testing.T) {
	resetUpgradeFlags(t)
	dir := setupV060Project(t)
	leoDir := filepath.Join(dir, ".leo")

	// Write a doc with type "metric".
	metricDoc := map[string]interface{}{
		"id":         "session-2026-04-10",
		"type":       "metric",
		"lifecycle":  "state",
		"scope":      "project",
		"tags":       []string{"metrics"},
		"created":    "2026-04-10T00:00:00Z",
		"created_by": "leo",
		"updated":    "2026-04-10T00:00:00Z",
		"updated_by": "leo",
		"content":    map[string]interface{}{"data": "test"},
	}
	docData, _ := json.MarshalIndent(metricDoc, "", "  ")
	os.WriteFile(filepath.Join(leoDir, "kb", "docs", "session-2026-04-10.json"), docData, 0644)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"upgrade"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}

	// Doc should now have type "session-log".
	data, _ := os.ReadFile(filepath.Join(leoDir, "kb", "docs", "session-2026-04-10.json"))
	if !strings.Contains(string(data), `"session-log"`) {
		t.Errorf("metric doc not migrated to session-log, got:\n%s", string(data))
	}
}

func TestUpgradeCmd_GeneratesRuntimeFiles(t *testing.T) {
	resetUpgradeFlags(t)
	dir := setupV060Project(t)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"upgrade"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}

	// CLAUDE.md should exist (claude is the migrated runtime).
	claudeMD := filepath.Join(dir, ".claude", "CLAUDE.md")
	if _, err := os.Stat(claudeMD); err != nil {
		t.Error("CLAUDE.md not generated during upgrade")
	}
}

// TestUpgradeCmd_GeneratedCLAUDEmd_NoRetiredContent verifies the generated
// CLAUDE.md does not contain any orchestration/profile references.
func TestUpgradeCmd_GeneratedCLAUDEmd_NoRetiredContent(t *testing.T) {
	resetUpgradeFlags(t)
	dir := setupV060Project(t)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"upgrade"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("upgrade failed: %v\noutput:\n%s", err, buf.String())
	}

	claudeMD := filepath.Join(dir, ".claude", "CLAUDE.md")
	data, err := os.ReadFile(claudeMD)
	if err != nil {
		t.Fatal("CLAUDE.md not found")
	}

	s := strings.ToLower(string(data))
	// These phrases indicate the retired orchestration model.
	forbidden := []string{"specialist", "delegation", "task-intake", "active profile",
		"orchestrates, never executes", "leo orchestrates", "task pipeline"}
	for _, bad := range forbidden {
		if strings.Contains(s, bad) {
			t.Errorf("CLAUDE.md must not contain %q after upgrade", bad)
		}
	}

	// Must contain communication mode directive.
	if !strings.Contains(string(data), "## Communication mode:") {
		t.Error("CLAUDE.md must contain communication mode section")
	}
}

func TestUpgradeCmd_OutputShowsActions(t *testing.T) {
	resetUpgradeFlags(t)
	dir := setupV060Project(t)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"upgrade"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "✔") {
		t.Error("expected checkmarks in upgrade output")
	}
	if !strings.Contains(out, "Upgrade complete") {
		t.Errorf("expected 'Upgrade complete' in output, got:\n%s", out)
	}
}
