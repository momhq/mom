package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestInitCmd_CreatesLeoStructure(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"init", "--runtime", "claude"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Verify .leo/ structure.
	expected := []string{
		".leo/config.yaml",
		".leo/identity.json",
		".leo/kb/schema.json",
		".leo/kb/index.json",
		".leo/profiles/general-manager.yaml",
		".leo/profiles/backend-engineer.yaml",
		".claude/CLAUDE.md",
		".leo/kb/constraints/anti-hallucination.json",
		".leo/kb/constraints/delegation-mandatory.json",
		".leo/kb/skills/session-wrap-up.json",
		".leo/kb/skills/task-intake.json",
	}

	for _, path := range expected {
		full := filepath.Join(dir, path)
		if _, err := os.Stat(full); err != nil {
			t.Errorf("missing expected file: %s", path)
		}
	}

	// Verify directories.
	dirs := []string{".leo/kb/docs", ".leo/kb/skills", ".leo/kb/constraints", ".leo/cache"}
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
	rootCmd.SetArgs([]string{"init", "--runtime", "claude"})

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
	rootCmd.SetArgs([]string{"init", "--runtime", "claude", "--force"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init --force failed: %v", err)
	}

	// Should have created the structure despite existing .leo/.
	if _, err := os.Stat(filepath.Join(dir, ".leo", "config.yaml")); err != nil {
		t.Error("config.yaml not created with --force")
	}
}
