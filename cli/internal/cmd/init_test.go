package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

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

	// Verify .leo/ structure.
	expected := []string{
		".leo/config.yaml",
		".leo/identity.json",
		".leo/kb/schema.json",
		".leo/kb/index.json",
		".leo/kb/logs",
		".claude/CLAUDE.md",
		".leo/kb/constraints/anti-hallucination.json",
		".leo/kb/skills/session-wrap-up.json",
	}
	// Retired files must NOT exist.
	retired := []string{
		".leo/profiles/general-manager.yaml",
		".leo/profiles/backend-engineer.yaml",
		".leo/kb/constraints/delegation-mandatory.json",
		".leo/kb/skills/task-intake.json",
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
	dirs := []string{".leo/kb/docs", ".leo/kb/skills", ".leo/kb/constraints", ".leo/kb/logs", ".leo/cache"}
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

func TestInitCmd_FailsIfAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	os.MkdirAll(filepath.Join(dir, ".leo"), 0755)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"init", "--runtimes", "claude"})

	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error when .leo/ already exists")
	}
}

func TestInitCmd_ForceOverwrite(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	os.MkdirAll(filepath.Join(dir, ".leo"), 0755)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"init", "--runtimes", "claude", "--force"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init --force failed: %v", err)
	}

	// Should have created the structure despite existing .leo/.
	if _, err := os.Stat(filepath.Join(dir, ".leo", "config.yaml")); err != nil {
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
	cfg, err := config.Load(filepath.Join(dir, ".leo"))
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}

	enabled := cfg.EnabledRuntimes()
	if len(enabled) != 3 {
		t.Errorf("expected 3 enabled runtimes, got %d: %v", len(enabled), enabled)
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
