package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryRegisterAndGet(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	a, ok := r.Get("claude")
	if !ok {
		t.Fatal("expected to find claude adapter")
	}
	if a.Name() != "claude" {
		t.Errorf("expected name 'claude', got %q", a.Name())
	}
}

func TestRegistryGetUnknown(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("expected false for unknown adapter")
	}
}

func TestRegistryDetectAll(t *testing.T) {
	dir := t.TempDir()

	// Create .claude/ and .clinerules/ but not AGENTS.md.
	// Note: .claude/ is shared by both claude and openclaude adapters.
	os.MkdirAll(filepath.Join(dir, ".claude"), 0755)
	os.MkdirAll(filepath.Join(dir, ".clinerules"), 0755)

	r := NewRegistry(dir)
	detected := r.DetectAll()

	if len(detected) != 3 {
		t.Fatalf("expected 3 detected adapters (claude, openclaude, cline), got %d", len(detected))
	}

	names := make(map[string]bool)
	for _, a := range detected {
		names[a.Name()] = true
	}
	if !names["claude"] {
		t.Error("expected claude to be detected")
	}
	if !names["openclaude"] {
		t.Error("expected openclaude to be detected")
	}
	if !names["cline"] {
		t.Error("expected cline to be detected")
	}
}

func TestRegistryAll(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	all := r.All()
	if len(all) != 5 {
		t.Fatalf("expected 5 adapters, got %d", len(all))
	}

	names := make(map[string]bool)
	for _, a := range all {
		names[a.Name()] = true
	}
	for _, expected := range []string{"claude", "codex", "cline", "openclaude", "windsurf"} {
		if !names[expected] {
			t.Errorf("expected %q in All()", expected)
		}
	}
}

