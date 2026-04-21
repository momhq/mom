package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vmarinogg/leo-core/cli/internal/adapters/runtime"
	"github.com/vmarinogg/leo-core/cli/internal/config"
)

func TestInitCmd_CreatesLeoStructure(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"init", "--runtimes", "claude"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Verify .mom/ structure.
	expected := []string{
		".mom/config.yaml",
		".mom/identity.json",
		".mom/schema.json",
		".mom/index.json",
		".mom/logs",
		".claude/CLAUDE.md",
		".mom/constraints/anti-hallucination.json",
		".mom/skills/session-wrap-up.json",
	}
	// Retired files must NOT exist.
	retired := []string{
		".mom/profiles/general-manager.yaml",
		".mom/profiles/backend-engineer.yaml",
		".mom/constraints/delegation-mandatory.json",
		".mom/skills/task-intake.json",
		".mom/kb",
	}
	for _, path := range retired {
		full := filepath.Join(dir, path)
		if _, err := os.Stat(full); err == nil {
			t.Errorf("retired file should not exist: %s", path)
		}
	}

	for _, path := range expected {
		full := filepath.Join(dir, path)
		if _, err := os.Stat(full); err != nil {
			t.Errorf("missing expected file: %s", path)
		}
	}

	// Verify directories.
	dirs := []string{".mom/memory", ".mom/skills", ".mom/constraints", ".mom/logs", ".mom/telemetry", ".mom/cache"}
	for _, d := range dirs {
		full := filepath.Join(dir, d)
		info, err := os.Stat(full)
		if err != nil {
			t.Errorf("missing expected dir: %s", d)
		} else if !info.IsDir() {
			t.Errorf("expected %s to be a directory", d)
		}
	}
}

func TestInitCmd_SkipsScaffoldIfAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	os.MkdirAll(filepath.Join(dir, ".mom"), 0755)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"init", "--runtimes", "claude"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected graceful skip when .mom/ already exists, got error: %v", err)
	}
	if !strings.Contains(buf.String(), "already exists") {
		t.Errorf("expected skip message in output, got: %s", buf.String())
	}
}

func TestInitCmd_ForceOverwrite(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	os.MkdirAll(filepath.Join(dir, ".mom"), 0755)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"init", "--runtimes", "claude", "--force"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init --force failed: %v", err)
	}

	// Should have created the structure despite existing .mom/.
	if _, err := os.Stat(filepath.Join(dir, ".mom", "config.yaml")); err != nil {
		t.Error("config.yaml not created with --force")
	}
}

func TestInitCmd_MultiRuntime(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"init", "--runtimes", "claude,codex,cline"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// All three runtime outputs should exist.
	files := map[string]string{
		".claude/CLAUDE.md":          "Claude",
		"AGENTS.md":                  "Codex",
		".clinerules/leo-context.md": "Cline",
	}

	for path, name := range files {
		full := filepath.Join(dir, path)
		if _, err := os.Stat(full); err != nil {
			t.Errorf("missing %s output: %s", name, path)
		}
	}

	// Config should have all three runtimes.
	cfg, err := config.Load(filepath.Join(dir, ".mom"))
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}

	enabled := cfg.EnabledRuntimes()
	if len(enabled) != 3 {
		t.Errorf("expected 3 enabled runtimes, got %d: %v", len(enabled), enabled)
	}
}

// TestInitCmd_ExperimentalWarning verifies that installing an adapter with
// experimental MRP events prints a user-visible warning.
func TestInitCmd_ExperimentalWarning(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"init", "--runtimes", "codex"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "⚠") {
		t.Errorf("expected experimental warning symbol in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Experimental") {
		t.Errorf("expected 'Experimental' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "best-effort") {
		t.Errorf("expected 'best-effort' message in output, got:\n%s", out)
	}
}

// TestInitCmd_NoWarningForClaudeOnly verifies that printExperimentalWarnings
// emits nothing when all selected adapters have no experimental events.
func TestInitCmd_NoWarningForClaudeOnly(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := &cobra.Command{}
	cmd.SetOut(buf)
	registry := runtime.NewRegistry(t.TempDir())
	printExperimentalWarnings(cmd, registry, []string{"claude"})

	out := buf.String()
	if strings.Contains(out, "best-effort") {
		t.Errorf("did not expect 'best-effort' warning for claude adapter, got:\n%s", out)
	}
}

func TestInitCmd_BackupExistingFile(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Create a user-owned AGENTS.md
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# My custom agents"), 0644)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"init", "--runtimes", "codex"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Original should be backed up
	bkpContent, err := os.ReadFile(filepath.Join(dir, "AGENTS.md.bkp"))
	if err != nil {
		t.Fatal("backup file not created")
	}
	if string(bkpContent) != "# My custom agents" {
		t.Error("backup content doesn't match original")
	}
}
