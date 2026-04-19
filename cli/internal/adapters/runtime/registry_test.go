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

	// Create .claude/ and .clinerules/ but not AGENTS.md
	os.MkdirAll(filepath.Join(dir, ".claude"), 0755)
	os.MkdirAll(filepath.Join(dir, ".clinerules"), 0755)

	r := NewRegistry(dir)
	detected := r.DetectAll()

	if len(detected) != 2 {
		t.Fatalf("expected 2 detected adapters, got %d", len(detected))
	}

	names := make(map[string]bool)
	for _, a := range detected {
		names[a.Name()] = true
	}
	if !names["claude"] {
		t.Error("expected claude to be detected")
	}
	if !names["cline"] {
		t.Error("expected cline to be detected")
	}
}

func TestRegistryAll(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	all := r.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 adapters, got %d", len(all))
	}

	names := make(map[string]bool)
	for _, a := range all {
		names[a.Name()] = true
	}
	for _, expected := range []string{"claude", "codex", "cline"} {
		if !names[expected] {
			t.Errorf("expected %q in All()", expected)
		}
	}
}

func TestRegistryGenerateAll(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(dir)

	config := Config{Version: "1", User: UserConfig{Language: "en", Mode: "concise", Autonomy: "balanced"}}

	err := r.GenerateAll([]string{"claude", "codex"}, config, nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateAll failed: %v", err)
	}

	// CLAUDE.md should exist
	if _, err := os.Stat(filepath.Join(dir, ".claude", "CLAUDE.md")); err != nil {
		t.Error("CLAUDE.md not generated")
	}

	// AGENTS.md should exist
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Error("AGENTS.md not generated")
	}

	// .clinerules/leo-context.md should NOT exist (cline not in enabled list)
	if _, err := os.Stat(filepath.Join(dir, ".clinerules", "leo-context.md")); err == nil {
		t.Error("leo-context.md should not have been generated")
	}
}
