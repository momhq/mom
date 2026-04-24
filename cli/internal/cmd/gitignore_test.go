package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/momhq/mom/cli/internal/adapters/runtime"
)

func TestEnsureGitIgnore_CreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	registry := runtime.NewRegistry(dir)

	added, err := ensureGitIgnore(dir, registry, []string{"claude"})
	if err != nil {
		t.Fatalf("ensureGitIgnore failed: %v", err)
	}

	if len(added) == 0 {
		t.Fatal("expected entries to be added")
	}

	content, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}

	s := string(content)
	// Core paths.
	if !strings.Contains(s, ".mom/") {
		t.Error("missing .mom/ in .gitignore")
	}
	if !strings.Contains(s, ".mcp.json") {
		t.Error("missing .mcp.json in .gitignore")
	}
	// Claude-specific paths.
	if !strings.Contains(s, ".claude/") {
		t.Error("missing .claude/ in .gitignore")
	}
	if !strings.Contains(s, "CLAUDE.md") {
		t.Error("missing CLAUDE.md in .gitignore")
	}
	// Header comment.
	if !strings.Contains(s, gitIgnoreHeader) {
		t.Error("missing header comment in .gitignore")
	}
}

func TestEnsureGitIgnore_AppendsToExisting(t *testing.T) {
	dir := t.TempDir()
	registry := runtime.NewRegistry(dir)

	existing := "node_modules/\n*.log\n"
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(existing), 0644)

	added, err := ensureGitIgnore(dir, registry, []string{"claude"})
	if err != nil {
		t.Fatalf("ensureGitIgnore failed: %v", err)
	}

	if len(added) == 0 {
		t.Fatal("expected entries to be added")
	}

	content, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}

	s := string(content)
	// Original content preserved.
	if !strings.HasPrefix(s, existing) {
		t.Error("original .gitignore content not preserved")
	}
	// MOM entries added.
	if !strings.Contains(s, ".mom/") {
		t.Error("missing .mom/ in .gitignore")
	}
}

func TestEnsureGitIgnore_NoDuplicates(t *testing.T) {
	dir := t.TempDir()
	registry := runtime.NewRegistry(dir)

	// Pre-populate with some MOM entries.
	existing := ".mom/\n.mcp.json\n"
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(existing), 0644)

	added, err := ensureGitIgnore(dir, registry, []string{"claude"})
	if err != nil {
		t.Fatalf("ensureGitIgnore failed: %v", err)
	}

	// Only Claude-specific paths should be added (not core paths).
	for _, entry := range added {
		if entry == ".mom/" || entry == ".mcp.json" {
			t.Errorf("should not have re-added existing entry: %s", entry)
		}
	}

	content, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}

	// Count occurrences of .mom/ — must be exactly 1.
	count := strings.Count(string(content), ".mom/")
	if count != 1 {
		t.Errorf("expected 1 occurrence of .mom/, got %d", count)
	}
}

func TestEnsureGitIgnore_AllPresent_NoOp(t *testing.T) {
	dir := t.TempDir()
	registry := runtime.NewRegistry(dir)

	existing := ".mom/\n.mcp.json\n.claude/\nCLAUDE.md\n"
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(existing), 0644)

	added, err := ensureGitIgnore(dir, registry, []string{"claude"})
	if err != nil {
		t.Fatalf("ensureGitIgnore failed: %v", err)
	}

	if len(added) != 0 {
		t.Errorf("expected no additions, got %v", added)
	}

	// File should be unchanged.
	content, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if string(content) != existing {
		t.Error("file was modified when all entries were already present")
	}
}

func TestEnsureGitIgnore_MultiRuntime(t *testing.T) {
	dir := t.TempDir()
	registry := runtime.NewRegistry(dir)

	added, err := ensureGitIgnore(dir, registry, []string{"claude", "codex", "windsurf"})
	if err != nil {
		t.Fatalf("ensureGitIgnore failed: %v", err)
	}

	if len(added) == 0 {
		t.Fatal("expected entries to be added")
	}

	content, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	s := string(content)

	expected := []string{".mom/", ".mcp.json", ".claude/", "CLAUDE.md", "AGENTS.md"}
	for _, entry := range expected {
		if !strings.Contains(s, entry) {
			t.Errorf("missing %s in .gitignore", entry)
		}
	}
}

func TestEnsureGitIgnore_Idempotent(t *testing.T) {
	dir := t.TempDir()
	registry := runtime.NewRegistry(dir)

	// First call.
	_, err := ensureGitIgnore(dir, registry, []string{"claude"})
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	contentAfterFirst, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))

	// Second call — should be a no-op.
	added, err := ensureGitIgnore(dir, registry, []string{"claude"})
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	if len(added) != 0 {
		t.Errorf("second call should add nothing, got %v", added)
	}

	contentAfterSecond, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if string(contentAfterFirst) != string(contentAfterSecond) {
		t.Error("file changed on second call")
	}
}

func TestEnsureGitIgnore_ExistingFileNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	registry := runtime.NewRegistry(dir)

	// File without trailing newline.
	existing := "node_modules/"
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(existing), 0644)

	_, err := ensureGitIgnore(dir, registry, []string{"claude"})
	if err != nil {
		t.Fatalf("ensureGitIgnore failed: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	s := string(content)

	// Should have a blank line separating old content from new block.
	if !strings.Contains(s, "node_modules/\n\n") {
		t.Errorf("expected blank line separator, got:\n%s", s)
	}
}

func TestDedup(t *testing.T) {
	input := []string{"a", "b", "a", "c", "b"}
	result := dedup(input)
	if len(result) != 3 {
		t.Errorf("expected 3, got %d: %v", len(result), result)
	}
	if result[0] != "a" || result[1] != "b" || result[2] != "c" {
		t.Errorf("unexpected order: %v", result)
	}
}
